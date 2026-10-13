package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/kimai"
)

// EventTrackerChanged is emitted with a TrackerState whenever the timer state changes.
const EventTrackerChanged = "tracker:changed"

// syncInterval is how often the running timer is reconciled with Kimai and
// how often an offline connection is re-checked.
const syncInterval = 60 * time.Second

// TrackerState is the timer state shown in the UI.
type TrackerState struct {
	Running     bool      `json:"running"`
	TimesheetID int       `json:"timesheetId"`
	Begin       string    `json:"begin,omitempty"` // RFC 3339
	Customer    Selection `json:"customer"`
	Project     Selection `json:"project"`
	Activity    Selection `json:"activity"`
	Description string    `json:"description"`
}

// StartRequest starts a new timer.
type StartRequest struct {
	Customer    Selection `json:"customer"`
	Project     Selection `json:"project"`
	Activity    Selection `json:"activity"`
	Description string    `json:"description"`
}

// TrackerService lists customers/projects/activities and starts/stops timers.
type TrackerService struct {
	conn     *Connection
	settings *SettingsService
	catalog  *Catalog

	mu     sync.Mutex
	state  TrackerState
	cancel context.CancelFunc
}

// NewTrackerService creates the service.
func NewTrackerService(conn *Connection, settings *SettingsService, catalog *Catalog) *TrackerService {
	return &TrackerService{conn: conn, settings: settings, catalog: catalog}
}

// ServiceStartup restores the last selection and starts background syncing.
func (t *TrackerService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	t.restoreSelection()
	ctx, t.cancel = context.WithCancel(ctx)
	go t.loop(ctx)
	return nil
}

// ServiceShutdown stops background syncing.
func (t *TrackerService) ServiceShutdown() error {
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

// loop keeps the timer in sync with Kimai (stopped in the web UI, started on
// another device) and re-checks the connection while offline.
func (t *TrackerService) loop(ctx context.Context) {
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			last := t.settings.LastConnection()
			if !last.OK {
				if last.Status == StatusNotConfigured {
					continue
				}
				if !t.settings.CheckConnection().OK {
					continue
				}
			}
			_, _ = t.Sync()
		}
	}
}

func (t *TrackerService) restoreSelection() {
	s := t.catalog.selection(t.baseURL())
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.state.Running {
		t.state.Customer, t.state.Project, t.state.Activity = s.Customer, s.Project, s.Activity
		t.state.Description = s.Description
	}
}

// State returns the current local state without contacting Kimai.
func (t *TrackerService) State() TrackerState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

// Sync fetches the running timer from Kimai and adopts it. A timer stopped
// elsewhere is reflected as stopped; the selection is kept.
func (t *TrackerService) Sync() (TrackerState, error) {
	client, err := t.client()
	if err != nil {
		return t.State(), err
	}
	ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
	defer cancel()
	active, err := client.ActiveTimesheets(ctx)
	if err != nil {
		return t.State(), t.fail(err)
	}
	t.mu.Lock()
	if len(active) == 0 {
		t.state.Running = false
		t.state.TimesheetID = 0
		t.state.Begin = ""
	} else {
		t.state = t.fromTimesheet(active[0], t.state)
	}
	st := t.state
	t.mu.Unlock()
	t.emit(st)
	return st, nil
}

// Customers lists customers. refresh bypasses the cache.
func (t *TrackerService) Customers(refresh bool) (OptionList, error) {
	return t.list(kindCustomers, 0, refresh, func(ctx context.Context, c *kimai.Client) ([]Option, error) {
		list, err := c.Customers(ctx)
		out := make([]Option, 0, len(list))
		for _, v := range list {
			out = append(out, Option{ID: v.ID, Name: v.Name, Hint: v.Number, Color: v.Color})
		}
		return out, err
	})
}

// Projects lists the projects of a customer.
func (t *TrackerService) Projects(customerID int, refresh bool) (OptionList, error) {
	return t.list(kindProjects, customerID, refresh, func(ctx context.Context, c *kimai.Client) ([]Option, error) {
		list, err := c.Projects(ctx, customerID)
		out := make([]Option, 0, len(list))
		for _, v := range list {
			out = append(out, Option{ID: v.ID, Name: v.Name, Color: v.Color})
		}
		return out, err
	})
}

// Activities lists the activities usable for a project (including global ones).
func (t *TrackerService) Activities(projectID int, refresh bool) (OptionList, error) {
	return t.list(kindActivities, projectID, refresh, func(ctx context.Context, c *kimai.Client) ([]Option, error) {
		list, err := c.Activities(ctx, projectID)
		out := make([]Option, 0, len(list))
		for _, v := range list {
			o := Option{ID: v.ID, Name: v.Name, Color: v.Color}
			if v.Project.ID == 0 {
				o.Hint = "global"
			}
			out = append(out, o)
		}
		return out, err
	})
}

func (t *TrackerService) list(kind string, parent int, refresh bool, fetch func(context.Context, *kimai.Client) ([]Option, error)) (OptionList, error) {
	base := t.baseURL()
	cached, haveCache, fresh := t.catalog.cached(base, kind, parent)
	if haveCache && fresh && !refresh {
		return OptionList{Items: cached}, nil
	}
	client, err := t.client()
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
		defer cancel()
		var items []Option
		items, err = fetch(ctx, client)
		if err == nil {
			return OptionList{Items: t.catalog.store(base, kind, parent, items)}, nil
		}
	}
	if haveCache {
		_ = t.fail(err)
		_, msg := DescribeError(err)
		return OptionList{Items: cached, Stale: true, Message: msg}, nil
	}
	return OptionList{}, t.fail(err)
}

// SaveSelection remembers the selection and draft description so they
// survive a restart. Ignored while a timer runs.
func (t *TrackerService) SaveSelection(customer, project, activity Selection, description string) {
	t.mu.Lock()
	if t.state.Running {
		t.mu.Unlock()
		return
	}
	t.state.Customer, t.state.Project, t.state.Activity = customer, project, activity
	t.state.Description = description
	t.mu.Unlock()
	t.catalog.saveSelection(t.baseURL(), customer, project, activity, description)
}

// Start starts a timer in Kimai.
func (t *TrackerService) Start(req StartRequest) (TrackerState, error) {
	if req.Project.ID == 0 || req.Activity.ID == 0 {
		return t.State(), errors.New("Bitte Kunde, Projekt und Tätigkeit auswählen.")
	}
	client, err := t.client()
	if err != nil {
		return t.State(), err
	}
	ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
	defer cancel()
	ts, err := client.StartTimesheet(ctx, kimai.StartInput{
		ProjectID:   req.Project.ID,
		ActivityID:  req.Activity.ID,
		Description: strings.TrimSpace(req.Description),
	})
	if err != nil {
		return t.State(), t.fail(err)
	}
	base := t.baseURL()
	t.catalog.markUsed(base, req.Customer, req.Project, req.Activity)
	t.catalog.saveSelection(base, req.Customer, req.Project, req.Activity, "")

	t.mu.Lock()
	t.state = t.fromTimesheet(ts, TrackerState{
		Customer: req.Customer, Project: req.Project, Activity: req.Activity,
	})
	st := t.state
	t.mu.Unlock()
	t.emit(st)
	return st, nil
}

// Stop stops the running timer. A timer already stopped or deleted in Kimai
// counts as stopped.
func (t *TrackerService) Stop() (TrackerState, error) {
	t.mu.Lock()
	id := t.state.TimesheetID
	t.mu.Unlock()
	if id == 0 {
		return t.Sync()
	}
	client, err := t.client()
	if err != nil {
		return t.State(), err
	}
	ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
	defer cancel()
	if _, err := client.StopTimesheet(ctx, id); err != nil && !errors.Is(err, kimai.ErrNotFound) {
		return t.State(), t.fail(err)
	}
	t.mu.Lock()
	t.state.Running = false
	t.state.TimesheetID = 0
	t.state.Begin = ""
	t.state.Description = ""
	st := t.state
	t.mu.Unlock()
	t.catalog.saveSelection(t.baseURL(), st.Customer, st.Project, st.Activity, "")
	t.emit(st)
	return st, nil
}

// SetDescription updates the description of the running timer in Kimai.
// Without a running timer it is kept as draft for the next start.
func (t *TrackerService) SetDescription(description string) (TrackerState, error) {
	t.mu.Lock()
	running, id := t.state.Running, t.state.TimesheetID
	t.mu.Unlock()
	if !running {
		st := t.State()
		t.SaveSelection(st.Customer, st.Project, st.Activity, description)
		return t.State(), nil
	}
	client, err := t.client()
	if err != nil {
		return t.State(), err
	}
	ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
	defer cancel()
	ts, err := client.UpdateDescription(ctx, id, description)
	if err != nil {
		return t.State(), t.fail(err)
	}
	t.mu.Lock()
	if t.state.TimesheetID == id {
		t.state.Description = ts.Description
	}
	st := t.state
	t.mu.Unlock()
	return st, nil
}

// fromTimesheet maps a Kimai timesheet onto the state, keeping known names
// when Kimai only returns IDs.
func (t *TrackerService) fromTimesheet(ts kimai.Timesheet, prev TrackerState) TrackerState {
	st := TrackerState{
		Running:     ts.Running(),
		TimesheetID: ts.ID,
		Description: ts.Description,
		Project:     Selection{ID: ts.Project.ID, Name: ts.Project.Name},
		Activity:    Selection{ID: ts.Activity.ID, Name: ts.Activity.Name},
	}
	if !ts.Begin.IsZero() {
		st.Begin = ts.Begin.Format(time.RFC3339)
	}
	if ts.Project.Customer != nil {
		st.Customer = Selection{ID: ts.Project.Customer.ID, Name: ts.Project.Customer.Name}
	}
	if st.Customer.ID == 0 && prev.Project.ID == st.Project.ID {
		st.Customer = prev.Customer
	}
	fill := func(sel *Selection, prevSel Selection, kind string) {
		if sel.Name != "" {
			return
		}
		if prevSel.ID == sel.ID && prevSel.Name != "" {
			sel.Name = prevSel.Name
		} else {
			sel.Name = t.catalog.name(kind, sel.ID)
		}
	}
	fill(&st.Customer, prev.Customer, kindCustomers)
	fill(&st.Project, prev.Project, kindProjects)
	fill(&st.Activity, prev.Activity, kindActivities)
	return st
}

// fail converts err for the UI. Connection problems (offline, token revoked)
// trigger a fresh connection check so the status and start button update.
func (t *TrackerService) fail(err error) error {
	if errors.Is(err, kimai.ErrOffline) || errors.Is(err, kimai.ErrUnauthorized) ||
		errors.Is(err, kimai.ErrCertificate) || errors.Is(err, context.DeadlineExceeded) {
		go t.settings.CheckConnection()
	}
	return userError(err)
}

func (t *TrackerService) client() (*kimai.Client, error) {
	c, err := t.conn.Client()
	if err != nil {
		return nil, userError(err)
	}
	return c, nil
}

func (t *TrackerService) baseURL() string {
	u, err := kimai.NormalizeBaseURL(t.conn.store.Get().KimaiURL)
	if err != nil {
		return ""
	}
	return u
}

func (t *TrackerService) emit(st TrackerState) {
	if a := application.Get(); a != nil {
		a.Event.Emit(EventTrackerChanged, st)
	}
}

// userError converts an error into one with a German, user-facing message.
func userError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNoToken) {
		return errors.New("Kein API-Token gespeichert. Bitte in den Einstellungen eintragen.")
	}
	_, msg := DescribeError(err)
	return fmt.Errorf("%s", msg)
}
