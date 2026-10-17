package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
	"github.com/klein-it-solutions/kimai-desktop-app/internal/kimai"
)

// EventPendingChanged is emitted with a PendingStatus whenever the number of
// entries waiting for Kimai changes.
const EventPendingChanged = "pending:changed"

// PendingEntry is a change saved while Kimai could not be reached. It is
// sent to Kimai once the connection is back.
//
//   - TimesheetID 0, End set: a new entry.
//   - TimesheetID 0, End zero: a timer started offline (still running).
//   - TimesheetID > 0: a change of an existing Kimai entry, e.g. a timer
//     stopped offline. A zero End leaves it running.
type PendingEntry struct {
	// ID is local and negative, so it never clashes with Kimai IDs.
	ID          int       `json:"id"`
	BaseURL     string    `json:"baseUrl"`
	TimesheetID int       `json:"timesheetId,omitempty"`
	Begin       time.Time `json:"begin"`
	End         time.Time `json:"end"`
	Customer    Selection `json:"customer"`
	Project     Selection `json:"project"`
	Activity    Selection `json:"activity"`
	Description string    `json:"description"`
	// Error is why Kimai rejected the entry on the last attempt.
	Error   string    `json:"error,omitempty"`
	SavedAt time.Time `json:"savedAt"`
}

// Running reports whether the entry has no end yet.
func (p PendingEntry) Running() bool { return p.End.IsZero() }

// isOfflineTimer reports whether the entry is a timer that only exists locally.
func (p PendingEntry) isOfflineTimer() bool { return p.TimesheetID == 0 && p.Running() }

// PendingStatus summarises the entries waiting for Kimai.
type PendingStatus struct {
	Count int `json:"count"`
	// Failed counts entries Kimai rejected; they need a correction.
	Failed int `json:"failed"`
}

type outboxFile struct {
	NextID  int            `json:"nextId"`
	Entries []PendingEntry `json:"entries"`
}

// Outbox keeps entries saved offline in ~/.config/kimai-tracker/pending.json.
// Unlike the list cache it is never discarded: entries of another Kimai URL
// are kept and synced when that URL is configured again.
type Outbox struct {
	path string
	// onChange is called after every change (outside the lock).
	onChange func()

	mu   sync.Mutex
	data outboxFile
}

// NewOutbox loads the outbox. An unreadable file is moved aside so the
// entries in it are not overwritten.
func NewOutbox(path string) *Outbox {
	o := &Outbox{path: path}
	if err := config.LoadJSON(path, &o.data); err != nil {
		log.Printf("outbox: %v – moving it to %s.broken", err, path)
		_ = os.Rename(path, path+".broken")
		o.data = outboxFile{}
	}
	return o
}

// list returns the entries of a Kimai instance: finished ones by begin,
// a running offline timer last (so it is started after the others).
func (o *Outbox) list(baseURL string) []PendingEntry {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []PendingEntry
	for _, p := range o.data.Entries {
		if p.BaseURL == baseURL {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Running() != out[j].Running() {
			return !out[i].Running()
		}
		return out[i].Begin.Before(out[j].Begin)
	})
	return out
}

func (o *Outbox) status(baseURL string) PendingStatus {
	var st PendingStatus
	for _, p := range o.list(baseURL) {
		st.Count++
		if p.Error != "" {
			st.Failed++
		}
	}
	return st
}

// get finds an entry by its local ID.
func (o *Outbox) get(baseURL string, id int) (PendingEntry, bool) {
	return o.find(func(p PendingEntry) bool { return p.BaseURL == baseURL && p.ID == id })
}

// forTimesheet finds the pending change of a Kimai entry.
func (o *Outbox) forTimesheet(baseURL string, timesheetID int) (PendingEntry, bool) {
	if timesheetID <= 0 {
		return PendingEntry{}, false
	}
	return o.find(func(p PendingEntry) bool { return p.BaseURL == baseURL && p.TimesheetID == timesheetID })
}

// offlineTimer returns the timer started offline, if any.
func (o *Outbox) offlineTimer(baseURL string) (PendingEntry, bool) {
	return o.find(func(p PendingEntry) bool { return p.BaseURL == baseURL && p.isOfflineTimer() })
}

func (o *Outbox) find(match func(PendingEntry) bool) (PendingEntry, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, p := range o.data.Entries {
		if match(p) {
			return p, true
		}
	}
	return PendingEntry{}, false
}

// put stores an entry. ID 0 adds a new one; a change of a Kimai entry
// replaces an earlier pending change of the same entry.
func (o *Outbox) put(p PendingEntry) (PendingEntry, error) {
	o.mu.Lock()
	next := o.data
	next.Entries = append([]PendingEntry(nil), o.data.Entries...)
	idx := -1
	for i, q := range next.Entries {
		if q.BaseURL != p.BaseURL {
			continue
		}
		if (p.ID != 0 && q.ID == p.ID) || (p.TimesheetID > 0 && q.TimesheetID == p.TimesheetID) {
			idx = i
			break
		}
	}
	switch {
	case idx >= 0:
		p.ID = next.Entries[idx].ID
		next.Entries[idx] = p
	case p.ID != 0:
		o.mu.Unlock()
		return PendingEntry{}, errors.New("Der Eintrag wurde inzwischen an Kimai übertragen. Bitte die Liste neu laden.")
	default:
		next.NextID++
		p.ID = -next.NextID
		next.Entries = append(next.Entries, p)
	}
	if err := config.SaveJSON(o.path, next); err != nil {
		o.mu.Unlock()
		return PendingEntry{}, fmt.Errorf("Eintrag konnte nicht lokal gespeichert werden: %v", err)
	}
	o.data = next
	o.mu.Unlock()
	o.changed()
	return p, nil
}

// setError remembers why Kimai rejected an entry.
func (o *Outbox) setError(baseURL string, id int, msg string) {
	o.mu.Lock()
	changed := false
	for i := range o.data.Entries {
		p := &o.data.Entries[i]
		if p.BaseURL == baseURL && p.ID == id && p.Error != msg {
			p.Error = msg
			changed = true
		}
	}
	if changed {
		_ = config.SaveJSON(o.path, o.data)
	}
	o.mu.Unlock()
	if changed {
		o.changed()
	}
}

// remove deletes an entry by its local ID.
func (o *Outbox) remove(baseURL string, id int) bool {
	return o.removeWhere(func(p PendingEntry) bool { return p.BaseURL == baseURL && p.ID == id })
}

// removeTimesheet drops a pending change of a Kimai entry (it was saved or
// deleted directly).
func (o *Outbox) removeTimesheet(baseURL string, timesheetID int) bool {
	if timesheetID <= 0 {
		return false
	}
	return o.removeWhere(func(p PendingEntry) bool { return p.BaseURL == baseURL && p.TimesheetID == timesheetID })
}

func (o *Outbox) removeWhere(match func(PendingEntry) bool) bool {
	o.mu.Lock()
	kept := o.data.Entries[:0:0]
	for _, p := range o.data.Entries {
		if !match(p) {
			kept = append(kept, p)
		}
	}
	removed := len(kept) != len(o.data.Entries)
	if removed {
		o.data.Entries = kept
		_ = config.SaveJSON(o.path, o.data)
	}
	o.mu.Unlock()
	if removed {
		o.changed()
	}
	return removed
}

func (o *Outbox) changed() {
	if o.onChange != nil {
		o.onChange()
	}
}

// isUnreachable reports errors after which an entry is kept locally instead:
// no network, a timeout or a proxy in front of Kimai that cannot reach it.
func isUnreachable(err error) bool {
	if errors.Is(err, kimai.ErrOffline) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr *kimai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
	}
	return false
}

// stopsSync reports errors that affect every request, so syncing the
// remaining entries is pointless (as opposed to Kimai rejecting one entry).
func stopsSync(err error) bool {
	return isUnreachable(err) ||
		errors.Is(err, kimai.ErrUnauthorized) ||
		errors.Is(err, kimai.ErrCertificate) ||
		errors.Is(err, kimai.ErrNotHTTPS) ||
		errors.Is(err, kimai.ErrInvalidURL) ||
		errors.Is(err, kimai.ErrUnexpectedResponse) ||
		errors.Is(err, ErrNoToken)
}
