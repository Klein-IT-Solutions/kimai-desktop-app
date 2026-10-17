package app

import (
	"strings"
	"testing"
	"time"
)

// setReachable records the connection status like the background check
// would, and disables the automatic re-check.
func setReachable(tr *TrackerService, ok bool) {
	tr.recheck = func() {}
	status := StatusOK
	if !ok {
		status = StatusOffline
	}
	tr.settings.mu.Lock()
	tr.settings.last = &ConnectionResult{OK: ok, Status: status}
	tr.settings.mu.Unlock()
}

func TestOfflineTimerIsSyncedLater(t *testing.T) {
	tr, f, _ := newTrackerFixture(t)
	tr.recheck = func() {}
	clock := time.Date(2026, 10, 13, 9, 0, 0, 0, time.Local)
	tr.now = func() time.Time { return clock }

	// Kimai is down: the start is attempted, fails and the timer runs locally.
	f.offline = true
	st, err := tr.Start(StartRequest{Customer: acmeSel, Project: webSel, Activity: devSel, Description: "Zug"})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || !st.Offline || st.TimesheetID >= 0 || st.Project.Name != "Web" {
		t.Fatalf("offline start = %+v", st)
	}
	setReachable(tr, false)
	calls := f.calls["POST /api/timesheets"]
	if st, err = tr.SetDescription("Zug nach Berlin"); err != nil || st.Description != "Zug nach Berlin" {
		t.Fatalf("offline description = %+v, %v", st, err)
	}
	if f.calls["POST /api/timesheets"] != calls || len(f.calls) > 2 {
		t.Errorf("no request expected while known offline: %v", f.calls)
	}
	if p := tr.Pending(); p.Count != 1 {
		t.Errorf("pending = %+v", p)
	}

	// The timer survives a restart.
	again := NewTrackerService(tr.conn, tr.settings, tr.catalog, NewOutbox(tr.outbox.path))
	again.restoreSelection()
	if st := again.State(); !st.Running || !st.Offline || st.Description != "Zug nach Berlin" {
		t.Fatalf("restored = %+v", st)
	}

	// Back online: the timer is created in Kimai with its original start.
	f.offline = false
	setReachable(tr, true)
	st, err = tr.Sync()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || st.Offline || st.TimesheetID <= 0 || st.Project.Name != "Web" || st.Customer.Name != "ACME" {
		t.Fatalf("synced = %+v", st)
	}
	if p := tr.Pending(); p.Count != 0 {
		t.Errorf("pending after sync = %+v", p)
	}
	if len(f.active) != 1 || f.active[st.TimesheetID]["description"] != "Zug nach Berlin" {
		t.Errorf("kimai = %v", f.active)
	}
}

func TestOfflineStopOfKimaiTimer(t *testing.T) {
	e, tr, f := newEntriesFixture(t)
	st, err := tr.Start(StartRequest{Customer: acmeSel, Project: webSel, Activity: devSel})
	if err != nil || st.TimesheetID <= 0 {
		t.Fatalf("start = %+v, %v", st, err)
	}
	id := st.TimesheetID
	f.offline = true
	setReachable(tr, false)
	tr.now = func() time.Time { return time.Date(2026, 10, 13, 11, 30, 0, 0, time.Local) }
	if st, err = tr.Stop(); err != nil || st.Running {
		t.Fatalf("offline stop = %+v, %v", st, err)
	}
	if p := tr.Pending(); p.Count != 1 {
		t.Errorf("pending = %+v", p)
	}

	// Online again: the stop is sent and Kimai's running timer is not adopted
	// again before that.
	f.offline = false
	setReachable(tr, true)
	if st, err = tr.Sync(); err != nil || st.Running {
		t.Fatalf("sync = %+v, %v", st, err)
	}
	if end, _ := f.sheets[id]["end"].(string); !strings.HasPrefix(end, "2026-10-13T11:30:00") {
		t.Errorf("stop not sent: %v", f.sheets[id])
	}
	day, err := e.Day("2026-10-13")
	if err != nil || len(day.Entries) != 1 || day.Entries[0].Running || day.Entries[0].Pending {
		t.Errorf("day = %+v, %v", day, err)
	}
}

func TestOfflineEntriesAreKeptAndSynced(t *testing.T) {
	e, tr, f := newEntriesFixture(t)
	f.offline = true
	tr.recheck = func() {}

	// The first save notices that Kimai is down and keeps the entry locally.
	added, err := e.Save(EntryInput{Date: "2026-10-13", Begin: "08:00", End: "09:30", Customer: acmeSel, Project: webSel, Activity: devSel, Description: "Konzept"})
	if err != nil {
		t.Fatal(err)
	}
	if added.ID >= 0 || !added.Pending || added.Duration != 5400 {
		t.Fatalf("added = %+v", added)
	}
	setReachable(tr, false)

	day, err := e.Day("2026-10-13")
	if err != nil {
		t.Fatal(err)
	}
	if !day.Offline || len(day.Entries) != 1 || !day.Entries[0].Pending || day.Total != 5400 {
		t.Fatalf("offline day = %+v", day)
	}

	// Change and delete work locally.
	if _, err := e.Save(EntryInput{ID: added.ID, Date: "2026-10-13", Begin: "08:00", End: "10:00", Customer: acmeSel, Project: webSel, Activity: devSel, Description: "Konzept"}); err != nil {
		t.Fatal(err)
	}
	other, err := e.Save(EntryInput{Date: "2026-10-13", Begin: "10:00", End: "10:15", Customer: acmeSel, Project: webSel, Activity: devSel})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Delete(other.ID); err != nil {
		t.Fatal(err)
	}
	if day, _ := e.Day("2026-10-13"); len(day.Entries) != 1 || day.Entries[0].End != "10:00" {
		t.Fatalf("after edit/delete = %+v", day)
	}
	if len(f.sheets) != 0 {
		t.Fatal("nothing must reach Kimai while offline")
	}

	// Online again: the entry is created once.
	f.offline = false
	setReachable(tr, true)
	if _, err := tr.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Sync(); err != nil {
		t.Fatal(err)
	}
	if end, _ := f.sheets[1]["end"].(string); len(f.sheets) != 1 || !strings.HasPrefix(end, "2026-10-13T10:00:00") {
		t.Errorf("kimai = %v", f.sheets)
	}
	day, err = e.Day("2026-10-13")
	if err != nil || day.Offline || len(day.Entries) != 1 || day.Entries[0].Pending || day.Entries[0].ID <= 0 {
		t.Errorf("online day = %+v, %v", day, err)
	}
	if p := tr.Pending(); p.Count != 0 {
		t.Errorf("pending = %+v", p)
	}
}

func TestRejectedOfflineEntryKeepsReason(t *testing.T) {
	e, tr, f := newEntriesFixture(t)
	f.offline = true
	setReachable(tr, false)
	added, err := e.Save(EntryInput{Date: "2026-10-13", Begin: "08:00", End: "09:00", Customer: acmeSel, Project: webSel, Activity: devSel})
	if err != nil {
		t.Fatal(err)
	}

	f.offline, f.reject = false, true
	setReachable(tr, true)
	if _, err := tr.Sync(); err != nil {
		t.Fatal(err)
	}
	if p := tr.Pending(); p.Count != 1 || p.Failed != 1 {
		t.Fatalf("pending = %+v", p)
	}
	day, _ := e.Day("2026-10-13")
	if len(day.Entries) != 1 || !strings.Contains(day.Entries[0].SyncError, "Ungültige Tätigkeit") {
		t.Fatalf("day = %+v", day)
	}

	// Correcting the entry sends it right away.
	f.reject = false
	if _, err := e.Save(EntryInput{ID: added.ID, Date: "2026-10-13", Begin: "08:00", End: "09:00", Customer: acmeSel, Project: webSel, Activity: devSel}); err != nil {
		t.Fatal(err)
	}
	if p := tr.Pending(); p.Count != 0 || len(f.sheets) != 1 {
		t.Errorf("pending = %+v, kimai = %v", p, f.sheets)
	}
}
