package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
)

// fakeEntries is an in-memory Kimai timesheet store for the entries service.
type fakeEntries struct {
	mu     sync.Mutex
	sheets map[int]map[string]any
	nextID int
	query  map[string]string
	last   map[string]any
	// offline answers every request with 502 (proxy cannot reach Kimai).
	offline bool
	// reject answers new entries with a validation error.
	reject bool
}

func newEntriesFixture(t *testing.T) (*EntriesService, *TrackerService, *fakeEntries) {
	t.Helper()
	f := &fakeEntries{sheets: map[int]map[string]any{}, nextID: 1}
	// Kimai answers in its time zone; the tests use the local one.
	stamp := func(v any) any {
		s, ok := v.(string)
		if !ok || s == "" {
			return nil
		}
		ts, _ := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local)
		return ts.Format("2006-01-02T15:04:05-0700")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.offline {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &body)
		}
		f.last = body
		var id int
		_, _ = fmt.Sscanf(r.URL.Path, "/api/timesheets/%d", &id)
		switch {
		case r.URL.Path == "/api/timesheets/active":
			list := []map[string]any{}
			for _, ts := range f.sheets {
				if ts["end"] == nil {
					list = append(list, ts)
				}
			}
			json.NewEncoder(w).Encode(list)
		case r.Method == "GET" && r.URL.Path == "/api/timesheets":
			f.query = map[string]string{}
			for k := range r.URL.Query() {
				f.query[k] = r.URL.Query().Get(k)
			}
			list := []map[string]any{}
			for i := 1; i < f.nextID; i++ {
				if ts, ok := f.sheets[i]; ok {
					list = append(list, ts)
				}
			}
			json.NewEncoder(w).Encode(list)
		case r.Method == "POST" && f.reject:
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"message":"Validation Failed","errors":{"children":{"activity":{"errors":["Ungültige Tätigkeit."]}}}}`)
		case r.Method == "POST":
			ts := map[string]any{"id": f.nextID, "begin": stamp(body["begin"]), "end": stamp(body["end"]),
				"project":  map[string]any{"id": body["project"], "name": "Web", "customer": map[string]any{"id": 1, "name": "ACME"}},
				"activity": map[string]any{"id": body["activity"], "name": "Dev"}, "description": body["description"]}
			f.sheets[f.nextID] = ts
			f.nextID++
			json.NewEncoder(w).Encode(ts)
		case r.Method == "PATCH":
			ts := f.sheets[id]
			ts["begin"] = stamp(body["begin"])
			if e, ok := body["end"]; ok {
				ts["end"] = stamp(e)
			}
			ts["description"] = body["description"]
			json.NewEncoder(w).Encode(ts)
		case r.Method == "DELETE":
			delete(f.sheets, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	store, err := config.Open(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	conn := NewConnection(store, config.MemorySecrets{}, &config.FileSecrets{Path: filepath.Join(dir, "secrets.json")})
	settings := NewSettingsService(conn)
	if _, err := settings.SaveSettings(SettingsInput{KimaiURL: srv.URL, Token: "tok"}); err != nil {
		t.Fatal(err)
	}
	tr := NewTrackerService(conn, settings, NewCatalog(filepath.Join(dir, "cache.json"), filepath.Join(dir, "state.json")), NewOutbox(filepath.Join(dir, "pending.json")))
	e := NewEntriesService(tr)
	e.now = func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.Local) }
	return e, tr, f
}

var (
	acmeSel = Selection{ID: 1, Name: "ACME"}
	webSel  = Selection{ID: 10, Name: "Web"}
	devSel  = Selection{ID: 100, Name: "Dev"}
)

func TestEntriesAddListEditDelete(t *testing.T) {
	e, _, f := newEntriesFixture(t)

	if _, err := e.Save(EntryInput{Date: "2026-10-13", Begin: "10:00", End: "09:00", Customer: acmeSel, Project: webSel, Activity: devSel}); err == nil || !strings.Contains(err.Error(), "Ende") {
		t.Errorf("end before begin: %v", err)
	}
	if _, err := e.Save(EntryInput{Date: "2026-10-13", Begin: "08:00", End: "09:30"}); err == nil {
		t.Error("missing project accepted")
	}

	added, err := e.Save(EntryInput{Date: "2026-10-13", Begin: "08:00", End: "09:30", Customer: acmeSel, Project: webSel, Activity: devSel, Description: " Konzept "})
	if err != nil {
		t.Fatal(err)
	}
	if added.ID == 0 || added.Begin != "08:00" || added.End != "09:30" || added.Duration != 5400 || added.Project.Name != "Web" {
		t.Errorf("added = %+v", added)
	}
	if f.last["description"] != "Konzept" || f.last["begin"] != "2026-10-13T08:00:00" || f.last["end"] != "2026-10-13T09:30:00" {
		t.Errorf("body = %v", f.last)
	}

	day, err := e.Day("2026-10-13")
	if err != nil {
		t.Fatal(err)
	}
	if !day.Today || day.Now != "12:00" || len(day.Entries) != 1 || day.Total != 5400 || day.Entries[0].Customer.Name != "ACME" {
		t.Errorf("day = %+v", day)
	}
	if f.query["begin"] != "2026-10-13T00:00:00" || f.query["end"] != "2026-10-13T23:59:59" {
		t.Errorf("query = %v", f.query)
	}

	added.Date, added.Begin, added.End = "2026-10-13", "08:15", "10:00"
	edited, err := e.Save(EntryInput{ID: added.ID, Date: added.Date, Begin: added.Begin, End: added.End, Customer: acmeSel, Project: webSel, Activity: devSel})
	if err != nil || edited.Begin != "08:15" || edited.End != "10:00" || edited.Duration != 6300 {
		t.Fatalf("edited = %+v, %v", edited, err)
	}

	if err := e.Delete(added.ID); err != nil {
		t.Fatal(err)
	}
	if day, _ := e.Day(""); len(day.Entries) != 0 || day.Date != "2026-10-13" {
		t.Errorf("after delete = %+v", day)
	}
}

func TestEntriesRunningStartTime(t *testing.T) {
	e, tr, f := newEntriesFixture(t)
	// Forgot to start: the timer runs since 11:40, the work began at 09:00.
	f.sheets[1] = map[string]any{"id": 1, "begin": time.Date(2026, 10, 13, 11, 40, 0, 0, time.Local).Format("2006-01-02T15:04:05-0700"), "end": nil,
		"project": 10, "activity": 100, "description": "läuft"}
	f.nextID = 2
	if st, err := tr.Sync(); err != nil || !st.Running {
		t.Fatalf("sync = %+v, %v", st, err)
	}

	if _, err := e.Save(EntryInput{ID: 1, Date: "2026-10-13", Begin: "12:30", Customer: acmeSel, Project: webSel, Activity: devSel}); err == nil {
		t.Error("start in the future accepted")
	}
	entry, err := e.Save(EntryInput{ID: 1, Date: "2026-10-13", Begin: "09:00", Customer: acmeSel, Project: webSel, Activity: devSel, Description: "läuft"})
	if err != nil {
		t.Fatal(err)
	}
	if _, sent := f.last["end"]; sent || !entry.Running || entry.Duration != 3*3600 {
		t.Errorf("entry = %+v, body = %v", entry, f.last)
	}
	if st := tr.State(); !st.Running || !strings.Contains(st.Begin, "T09:00:00") {
		t.Errorf("tracker = %+v", st)
	}

	// Giving the running entry an end stops the timer.
	if _, err := e.Save(EntryInput{ID: 1, Date: "2026-10-13", Begin: "09:00", End: "11:45", Customer: acmeSel, Project: webSel, Activity: devSel}); err != nil {
		t.Fatal(err)
	}
	if tr.State().Running {
		t.Error("timer should be stopped")
	}
}
