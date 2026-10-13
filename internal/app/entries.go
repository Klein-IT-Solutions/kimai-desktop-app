package app

import (
	"context"
	"errors"
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
}

// DayEntries lists the entries of one day.
type DayEntries struct {
	Date    string  `json:"date"`
	Today   bool    `json:"today"`
	Now     string  `json:"now"` // current time (HH:MM) in the Kimai time zone
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"` // seconds
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
	ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
	defer cancel()
	list, err := client.Timesheets(ctx, day, day.AddDate(0, 0, 1).Add(-time.Second))
	if err != nil {
		return DayEntries{}, e.tracker.fail(err)
	}
	out := DayEntries{Date: date, Today: date == today, Now: now.Format(clockLayout), Entries: []Entry{}}
	for _, ts := range list {
		entry := e.entry(ts, loc, now)
		out.Entries = append(out.Entries, entry)
		out.Total += entry.Duration
	}
	return out, nil
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
		return Entry{}, e.tracker.fail(err)
	}
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

// Delete removes an entry from Kimai.
func (e *EntriesService) Delete(id int) error {
	client, err := e.tracker.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
	defer cancel()
	if err := client.DeleteTimesheet(ctx, id); err != nil && !errors.Is(err, kimai.ErrNotFound) {
		return e.tracker.fail(err)
	}
	if id == e.tracker.State().TimesheetID {
		_, _ = e.tracker.Sync()
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
