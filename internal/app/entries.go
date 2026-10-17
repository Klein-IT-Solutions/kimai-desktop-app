package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/kimai"
)

const (
	dateLayout  = "2006-01-02"
	clockLayout = "15:04"
)

// Entry is a timesheet in the entry list and editor. Date and times are in
// the Kimai user's time zone.
type Entry struct {
	ID          int       `json:"id"`
	Date        string    `json:"date"`     // 2026-10-13
	Begin       string    `json:"begin"`    // 09:00
	End         string    `json:"end"`      // "" while running
	Duration    int       `json:"duration"` // seconds; running entries count until now
	Running     bool      `json:"running"`
	Customer    Selection `json:"customer"`
	Project     Selection `json:"project"`
	Activity    Selection `json:"activity"`
	Description string    `json:"description"`
	// Pending marks entries saved locally that are not in Kimai yet.
	Pending bool `json:"pending"`
	// SyncError is why Kimai rejected a pending entry.
	SyncError string `json:"syncError,omitempty"`
}

// DayEntries lists the entries of one day.
type DayEntries struct {
	Date    string  `json:"date"`
	Today   bool    `json:"today"`
	Now     string  `json:"now"` // current time (HH:MM) in the Kimai time zone
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"` // seconds
	// Offline is set when Kimai could not be reached: only entries saved
	// locally (and a running timer) are listed.
	Offline bool `json:"offline"`
}

// EntryInput creates (ID 0) or changes an entry. End may only be empty for
// the running entry, which then keeps running.
type EntryInput struct {
	ID          int       `json:"id"`
	Date        string    `json:"date"`
	Begin       string    `json:"begin"`
	End         string    `json:"end"`
	Customer    Selection `json:"customer"`
	Project     Selection `json:"project"`
	Activity    Selection `json:"activity"`
	Description string    `json:"description"`
}

// EntriesService lists, adds, changes and deletes timesheets afterwards –
// for a forgotten start or entering the day in the evening.
type EntriesService struct {
	tracker *TrackerService
	now     func() time.Time
}

// NewEntriesService creates the service; it shares connection, catalog and
// timer state with the tracker.
func NewEntriesService(tracker *TrackerService) *EntriesService {
	return &EntriesService{tracker: tracker, now: time.Now}
}

// Day returns the entries of a day (YYYY-MM-DD; empty means today).
func (e *EntriesService) Day(date string) (DayEntries, error) {
	client, err := e.tracker.client()
	if err != nil {
		return DayEntries{}, err
	}
	loc := clientLocation(client)
	now := e.now().In(loc)
	today := now.Format(dateLayout)
	if date == "" {
		date = today
	}
	day, err := time.ParseInLocation(dateLayout, date, loc)
	if err != nil {
		return DayEntries{}, errors.New("Ungültiges Datum.")
	}
	out := DayEntries{Date: date, Today: date == today, Now: now.Format(clockLayout), Entries: []Entry{}}
	var list []kimai.Timesheet
	if e.tracker.settings.knownOffline() {
		out.Offline = true
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
		defer cancel()
		list, err = client.Timesheets(ctx, day, day.AddDate(0, 0, 1).Add(-time.Second))
		if err != nil {
			if !isUnreachable(err) {
				return DayEntries{}, e.tracker.fail(err)
			}
			_ = e.tracker.fail(err)
			out.Offline = true
		}
	}

	// Local changes replace what Kimai knows; new local entries are added.
	base := e.tracker.baseURL()
	pending := map[int]PendingEntry{}
	for _, p := range e.tracker.outbox.list(base) {
		if p.Begin.In(loc).Format(dateLayout) == date {
			pending[p.ID] = p
		}
	}
	for _, ts := range list {
		if p, ok := e.tracker.outbox.forTimesheet(base, ts.ID); ok {
			delete(pending, p.ID)
			if p.Begin.In(loc).Format(dateLayout) != date {
				continue // moved to another day
			}
			out.Entries = append(out.Entries, pendingEntry(p, loc, now))
			continue
		}
		out.Entries = append(out.Entries, e.entry(ts, loc, now))
	}
	if out.Offline {
		// Kimai's entries are unknown, but the running timer is.
		if st := e.tracker.State(); st.Running && st.TimesheetID > 0 {
			if _, ok := e.tracker.outbox.forTimesheet(base, st.TimesheetID); !ok {
				if begin, err := time.Parse(time.RFC3339, st.Begin); err == nil && begin.In(loc).Format(dateLayout) == date {
					out.Entries = append(out.Entries, pendingEntry(PendingEntry{
						TimesheetID: st.TimesheetID, Begin: begin, Customer: st.Customer,
						Project: st.Project, Activity: st.Activity, Description: st.Description,
					}, loc, now))
					out.Entries[len(out.Entries)-1].Pending = false
				}
			}
		}
	}
	for _, p := range pending {
		out.Entries = append(out.Entries, pendingEntry(p, loc, now))
	}
	sort.SliceStable(out.Entries, func(i, j int) bool { return out.Entries[i].Begin < out.Entries[j].Begin })
	for _, entry := range out.Entries {
		out.Total += entry.Duration
	}
	return out, nil
}

// pendingEntry converts a locally saved entry for the list. Changes of a
// Kimai entry keep its Kimai ID, so editing them works as usual.
func pendingEntry(p PendingEntry, loc *time.Location, now time.Time) Entry {
	id := p.ID
	if p.TimesheetID > 0 {
		id = p.TimesheetID
	}
	begin := p.Begin.In(loc)
	entry := Entry{
		ID:          id,
		Date:        begin.Format(dateLayout),
		Begin:       begin.Format(clockLayout),
		Running:     p.Running(),
		Customer:    p.Customer,
		Project:     p.Project,
		Activity:    p.Activity,
		Description: p.Description,
		Pending:     true,
		SyncError:   p.Error,
	}
	if entry.Running {
		entry.Duration = max(0, int(now.Sub(p.Begin).Seconds()))
	} else {
		entry.End = p.End.In(loc).Format(clockLayout)
		entry.Duration = max(0, int(p.End.Sub(p.Begin).Seconds()))
	}
	return entry
}

// Save creates or updates an entry in Kimai.
func (e *EntriesService) Save(in EntryInput) (Entry, error) {
	if in.Project.ID == 0 || in.Activity.ID == 0 {
		return Entry{}, errors.New("Bitte Kunde, Projekt und Tätigkeit auswählen.")
	}
	client, err := e.tracker.client()
	if err != nil {
		return Entry{}, err
	}
	loc := clientLocation(client)
	now := e.now().In(loc)

	begin, err := parseClock(in.Date, in.Begin, loc)
	if err != nil {
		return Entry{}, errors.New("Bitte eine gültige Startzeit angeben.")
	}
	runningID := e.tracker.State().TimesheetID
	keepRunning := in.ID != 0 && in.ID == runningID && strings.TrimSpace(in.End) == ""
	var end time.Time
	if keepRunning {
		if begin.After(now) {
			return Entry{}, errors.New("Die Startzeit liegt in der Zukunft.")
		}
	} else {
		if end, err = parseClock(in.Date, in.End, loc); err != nil {
			return Entry{}, errors.New("Bitte eine gültige Endzeit angeben.")
		}
		if !end.After(begin) {
			return Entry{}, errors.New("Das Ende muss nach dem Beginn liegen.")
		}
	}

	if offline := e.tracker.settings.knownOffline(); in.ID < 0 || offline {
		return e.saveLocally(in, begin, end, loc, now, !offline)
	}
	input := kimai.EntryInput{
		ProjectID:   in.Project.ID,
		ActivityID:  in.Activity.ID,
		Description: strings.TrimSpace(in.Description),
		Begin:       begin,
		End:         end,
	}
	ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
	defer cancel()
	var ts kimai.Timesheet
	if in.ID == 0 {
		ts, err = client.CreateTimesheet(ctx, input)
	} else {
		ts, err = client.UpdateTimesheet(ctx, in.ID, input)
	}
	if err != nil {
		if isUnreachable(err) {
			_ = e.tracker.fail(err)
			return e.saveLocally(in, begin, end, loc, now, false)
		}
		return Entry{}, e.tracker.fail(err)
	}
	// An older local change of this entry is obsolete now.
	e.tracker.outbox.removeTimesheet(e.tracker.baseURL(), in.ID)
	e.tracker.catalog.markUsed(e.tracker.baseURL(), in.Customer, in.Project, in.Activity)
	if in.ID != 0 && in.ID == runningID {
		// Stopped by giving it an end, or changed: refresh the timer.
		_, _ = e.tracker.Sync()
	}

	// Kimai may answer with plain IDs; keep the names the user picked.
	entry := e.entry(ts, loc, now)
	entry.Customer, entry.Project, entry.Activity = in.Customer, in.Project, in.Activity
	return entry, nil
}

// saveLocally keeps a new or changed entry until Kimai is reachable again.
// With trySync the outbox is sent right away (Kimai seems reachable).
func (e *EntriesService) saveLocally(in EntryInput, begin, end time.Time, loc *time.Location, now time.Time, trySync bool) (Entry, error) {
	t := e.tracker
	t.outboxMu.Lock()
	base := t.baseURL()
	p := PendingEntry{
		BaseURL:     base,
		Begin:       begin,
		End:         end,
		Customer:    in.Customer,
		Project:     in.Project,
		Activity:    in.Activity,
		Description: strings.TrimSpace(in.Description),
		SavedAt:     now,
	}
	switch {
	case in.ID < 0:
		old, ok := t.outbox.get(base, in.ID)
		if !ok {
			t.outboxMu.Unlock()
			return Entry{}, errors.New("Der Eintrag wurde inzwischen an Kimai übertragen. Bitte die Liste neu laden.")
		}
		p.ID, p.TimesheetID = old.ID, old.TimesheetID
	case in.ID > 0:
		p.TimesheetID = in.ID
	}
	p, err := t.outbox.put(p)
	t.outboxMu.Unlock()
	if err != nil {
		return Entry{}, err
	}
	t.catalog.markUsed(base, in.Customer, in.Project, in.Activity)

	if st := t.State(); in.ID != 0 && in.ID == st.TimesheetID && st.Running {
		if p.Running() {
			t.mu.Lock()
			if t.state.TimesheetID == in.ID {
				t.state.Begin = begin.Format(time.RFC3339)
				t.state.Customer, t.state.Project, t.state.Activity = in.Customer, in.Project, in.Activity
				t.state.Description = p.Description
			}
			st = t.state
			t.mu.Unlock()
			t.emit(st)
		} else {
			t.stopped()
		}
	}
	// E.g. a rejected entry was corrected: send it right away.
	if trySync {
		_, _ = t.Sync()
	}
	return pendingEntry(p, loc, now), nil
}

// Delete removes an entry from Kimai, or a locally saved one.
func (e *EntriesService) Delete(id int) error {
	t := e.tracker
	base := t.baseURL()
	if id < 0 {
		t.outboxMu.Lock()
		t.outbox.remove(base, id)
		t.outboxMu.Unlock()
		if st := t.State(); st.Running && st.TimesheetID == id {
			t.stopped()
		}
		return nil
	}
	client, err := t.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
	defer cancel()
	if err := client.DeleteTimesheet(ctx, id); err != nil && !errors.Is(err, kimai.ErrNotFound) {
		if isUnreachable(err) {
			_ = t.fail(err)
			return errors.New("Kimai ist nicht erreichbar. Offline lassen sich nur Einträge löschen, die noch nicht übertragen wurden.")
		}
		return t.fail(err)
	}
	t.outboxMu.Lock()
	t.outbox.removeTimesheet(base, id)
	t.outboxMu.Unlock()
	if id == t.State().TimesheetID {
		_, _ = t.Sync()
	}
	return nil
}

func (e *EntriesService) entry(ts kimai.Timesheet, loc *time.Location, now time.Time) Entry {
	sel := e.tracker.fromTimesheet(ts, TrackerState{})
	begin := ts.Begin.In(loc)
	entry := Entry{
		ID:          ts.ID,
		Date:        begin.Format(dateLayout),
		Begin:       begin.Format(clockLayout),
		Running:     ts.Running(),
		Customer:    sel.Customer,
		Project:     sel.Project,
		Activity:    sel.Activity,
		Description: ts.Description,
		Duration:    ts.Duration,
	}
	if entry.Running {
		entry.Duration = max(0, int(now.Sub(ts.Begin.Time).Seconds()))
	} else {
		entry.End = ts.End.In(loc).Format(clockLayout)
		if entry.Duration == 0 {
			entry.Duration = int(ts.End.Sub(ts.Begin.Time).Seconds())
		}
	}
	return entry
}

func parseClock(date, clock string, loc *time.Location) (time.Time, error) {
	return time.ParseInLocation(dateLayout+" "+clockLayout, strings.TrimSpace(date)+" "+strings.TrimSpace(clock), loc)
}

func clientLocation(c *kimai.Client) *time.Location {
	if c.Location != nil {
		return c.Location
	}
	return time.Local
}
