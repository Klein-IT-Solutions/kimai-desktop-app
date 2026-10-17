package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/kimai"
)

// Pending returns how many entries wait to be sent to Kimai.
func (t *TrackerService) Pending() PendingStatus {
	return t.outbox.status(t.baseURL())
}

func (t *TrackerService) emitPending() {
	if a := application.Get(); a != nil {
		a.Event.Emit(EventPendingChanged, t.Pending())
	}
}

// offlineState is the timer state of a timer started offline.
func offlineState(p PendingEntry) TrackerState {
	return TrackerState{
		Running:     true,
		Offline:     true,
		TimesheetID: p.ID,
		Begin:       p.Begin.Format(time.RFC3339),
		Customer:    p.Customer,
		Project:     p.Project,
		Activity:    p.Activity,
		Description: p.Description,
	}
}

// startOffline starts a timer that only exists locally until Kimai is
// reachable again.
func (t *TrackerService) startOffline(req StartRequest) (TrackerState, error) {
	t.outboxMu.Lock()
	defer t.outboxMu.Unlock()
	base := t.baseURL()
	now := t.now().Truncate(time.Second)
	p, err := t.outbox.put(PendingEntry{
		BaseURL:     base,
		Begin:       now,
		Customer:    req.Customer,
		Project:     req.Project,
		Activity:    req.Activity,
		Description: strings.TrimSpace(req.Description),
		SavedAt:     now,
	})
	if err != nil {
		return t.State(), err
	}
	t.catalog.markUsed(base, req.Customer, req.Project, req.Activity)
	t.catalog.saveSelection(base, req.Customer, req.Project, req.Activity, "")

	t.mu.Lock()
	t.state = offlineState(p)
	st := t.state
	t.mu.Unlock()
	t.emit(st)
	return st, nil
}

// stopLocally stops a timer started offline; it becomes a pending entry.
// Must be called with outboxMu held.
func (t *TrackerService) stopLocally(st TrackerState) (TrackerState, error) {
	base := t.baseURL()
	if p, ok := t.outbox.get(base, st.TimesheetID); ok {
		p.End = t.now().Truncate(time.Second)
		p.Description = strings.TrimSpace(st.Description)
		p.Error = ""
		if _, err := t.outbox.put(p); err != nil {
			return t.State(), err
		}
	}
	return t.stopped(), nil
}

// stopQueued stops a Kimai timer while Kimai is unreachable: the end time is
// kept locally and sent later.
func (t *TrackerService) stopQueued(st TrackerState) (TrackerState, error) {
	begin, err := time.Parse(time.RFC3339, st.Begin)
	if err != nil {
		return t.State(), errors.New("Kimai ist nicht erreichbar und die Startzeit des Timers ist unbekannt.")
	}
	t.outboxMu.Lock()
	defer t.outboxMu.Unlock()
	_, err = t.outbox.put(PendingEntry{
		BaseURL:     t.baseURL(),
		TimesheetID: st.TimesheetID,
		Begin:       begin,
		End:         t.now().Truncate(time.Second),
		Customer:    st.Customer,
		Project:     st.Project,
		Activity:    st.Activity,
		Description: strings.TrimSpace(st.Description),
		SavedAt:     t.now(),
	})
	if err != nil {
		return t.State(), err
	}
	return t.stopped(), nil
}

// setOfflineDescription changes the description of a timer started offline.
// done is false when the timer has been sent to Kimai in the meantime.
func (t *TrackerService) setOfflineDescription(id int, description string) (st TrackerState, done bool, err error) {
	t.outboxMu.Lock()
	defer t.outboxMu.Unlock()
	if t.State().TimesheetID != id {
		return TrackerState{}, false, nil
	}
	if p, ok := t.outbox.get(t.baseURL(), id); ok {
		p.Description = strings.TrimSpace(description)
		p.Error = ""
		if _, err := t.outbox.put(p); err != nil {
			return t.State(), true, err
		}
	}
	return t.setStateDescription(id, description), true, nil
}

// queueDescription keeps the description of a running Kimai timer locally
// while Kimai is unreachable.
func (t *TrackerService) queueDescription(id int, description string) (TrackerState, error) {
	st := t.State()
	begin, err := time.Parse(time.RFC3339, st.Begin)
	if err != nil || st.TimesheetID != id {
		return st, errors.New("Kimai ist nicht erreichbar. Die Beschreibung wird gespeichert, sobald die Verbindung wieder da ist.")
	}
	t.outboxMu.Lock()
	defer t.outboxMu.Unlock()
	_, err = t.outbox.put(PendingEntry{
		BaseURL:     t.baseURL(),
		TimesheetID: id,
		Begin:       begin,
		Customer:    st.Customer,
		Project:     st.Project,
		Activity:    st.Activity,
		Description: strings.TrimSpace(description),
		SavedAt:     t.now(),
	})
	if err != nil {
		return st, err
	}
	return t.setStateDescription(id, description), nil
}

func (t *TrackerService) setStateDescription(id int, description string) TrackerState {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state.TimesheetID == id {
		t.state.Description = description
	}
	return t.state
}

// flush sends the outbox to Kimai, oldest entry first. It stops at the
// first problem that affects every request (e.g. offline again); entries
// Kimai rejects keep the reason and are retried with the next sync.
func (t *TrackerService) flush(client *kimai.Client) error {
	t.outboxMu.Lock()
	defer t.outboxMu.Unlock()
	base := t.baseURL()
	for _, p := range t.outbox.list(base) {
		ts, err := pushEntry(client, p)
		if err != nil {
			if stopsSync(err) {
				return err
			}
			t.outbox.setError(base, p.ID, rejectionMessage(p, err))
			continue
		}
		t.outbox.remove(base, p.ID)
		if p.isOfflineTimer() {
			t.adoptSynced(p, ts)
		}
	}
	return nil
}

func pushEntry(client *kimai.Client, p PendingEntry) (kimai.Timesheet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
	defer cancel()
	in := kimai.EntryInput{
		ProjectID:   p.Project.ID,
		ActivityID:  p.Activity.ID,
		Description: strings.TrimSpace(p.Description),
		Begin:       p.Begin,
		End:         p.End,
	}
	switch {
	case p.TimesheetID > 0:
		return client.UpdateTimesheet(ctx, p.TimesheetID, in)
	case p.Running():
		return client.StartTimesheet(ctx, kimai.StartInput{
			ProjectID: in.ProjectID, ActivityID: in.ActivityID, Description: in.Description, Begin: in.Begin,
		})
	default:
		return client.CreateTimesheet(ctx, in)
	}
}

func rejectionMessage(p PendingEntry, err error) string {
	if p.TimesheetID > 0 && errors.Is(err, kimai.ErrNotFound) {
		return "Der Eintrag existiert in Kimai nicht mehr. Bitte lokal löschen."
	}
	_, msg := DescribeError(err)
	return msg
}

// adoptSynced replaces the local timer with the one Kimai just created.
func (t *TrackerService) adoptSynced(p PendingEntry, ts kimai.Timesheet) {
	t.mu.Lock()
	if !t.state.Offline || t.state.TimesheetID != p.ID {
		t.mu.Unlock()
		return
	}
	t.state = t.fromTimesheet(ts, t.state)
	st := t.state
	t.mu.Unlock()
	t.emit(st)
}
