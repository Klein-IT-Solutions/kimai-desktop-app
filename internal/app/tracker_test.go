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

	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
)

// fakeTimesheets is a tiny in-memory Kimai for tracker tests.
type fakeTimesheets struct {
	mu      sync.Mutex
	active  map[int]map[string]any
	nextID  int
	offline bool
	calls   map[string]int
}

func newTrackerFixture(t *testing.T) (*TrackerService, *fakeTimesheets, *httptest.Server) {
	t.Helper()
	f := &fakeTimesheets{active: map[int]map[string]any{}, nextID: 100, calls: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls[r.Method+" "+r.URL.Path]++
		if f.offline {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &body)
		}
		switch {
		case r.URL.Path == "/api/customers":
			io.WriteString(w, `[{"id":1,"name":"ACME"},{"id":2,"name":"Beta"}]`)
		case r.URL.Path == "/api/projects":
			io.WriteString(w, `[{"id":10,"name":"Web","customer":1}]`)
		case r.URL.Path == "/api/activities":
			io.WriteString(w, `[{"id":100,"name":"Dev","project":10},{"id":101,"name":"Meeting","project":null}]`)
		case r.URL.Path == "/api/timesheets/active":
			var list []map[string]any
			for _, ts := range f.active {
				list = append(list, ts)
			}
			json.NewEncoder(w).Encode(list)
		case r.Method == "POST" && r.URL.Path == "/api/timesheets":
			f.nextID++
			ts := map[string]any{"id": f.nextID, "begin": "2026-10-13T10:00:00+0200", "end": nil,
				"project": body["project"], "activity": body["activity"], "description": body["description"]}
			f.active[f.nextID] = ts
			json.NewEncoder(w).Encode(ts)
		case r.Method == "PATCH" && strings.HasSuffix(r.URL.Path, "/stop"):
			var id int
			_, _ = fmt.Sscanf(r.URL.Path, "/api/timesheets/%d/stop", &id)
			ts, ok := f.active[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				io.WriteString(w, `{"message":"not found"}`)
				return
			}
			delete(f.active, id)
			ts["end"] = "2026-10-13T11:00:00+0200"
			json.NewEncoder(w).Encode(ts)
		case r.Method == "PATCH":
			var id int
			_, _ = fmt.Sscanf(r.URL.Path, "/api/timesheets/%d", &id)
			ts := f.active[id]
			ts["description"] = body["description"]
			json.NewEncoder(w).Encode(ts)
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
	secrets := config.MemorySecrets{}
	conn := NewConnection(store, secrets, &config.FileSecrets{Path: filepath.Join(dir, "secrets.json")})
	settings := NewSettingsService(conn)
	if _, err := settings.SaveSettings(SettingsInput{KimaiURL: srv.URL, Token: "tok"}); err != nil {
		t.Fatal(err)
	}
	catalog := NewCatalog(filepath.Join(dir, "cache.json"), filepath.Join(dir, "state.json"))
	return NewTrackerService(conn, settings, catalog), f, srv
}

func TestTrackerStartStopAndDescription(t *testing.T) {
	tr, f, _ := newTrackerFixture(t)
	acme, web, dev := Selection{ID: 1, Name: "ACME"}, Selection{ID: 10, Name: "Web"}, Selection{ID: 100, Name: "Dev"}

	st, err := tr.Start(StartRequest{Customer: acme, Project: web, Activity: dev, Description: " Hallo "})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || st.TimesheetID == 0 || st.Customer.Name != "ACME" || st.Project.Name != "Web" || st.Activity.Name != "Dev" {
		t.Fatalf("started = %+v", st)
	}
	if st.Description != "Hallo" || st.Begin == "" {
		t.Errorf("description/begin = %q / %q", st.Description, st.Begin)
	}

	st, err = tr.SetDescription("Neu")
	if err != nil || st.Description != "Neu" {
		t.Fatalf("SetDescription = %+v, %v", st, err)
	}

	st, err = tr.Stop()
	if err != nil || st.Running || st.TimesheetID != 0 {
		t.Fatalf("stopped = %+v, %v", st, err)
	}
	if st.Project.ID != 10 {
		t.Error("selection should be kept after stop")
	}
	if len(f.active) != 0 {
		t.Error("timesheet still active in Kimai")
	}

	// Recently used entries move to the top.
	list, err := tr.Customers(false)
	if err != nil || list.Items[0].Name != "ACME" || !list.Items[0].Recent {
		t.Errorf("customers = %+v, %v", list, err)
	}
}

func TestTrackerSyncAdoptsAndReleasesExternalTimer(t *testing.T) {
	tr, f, _ := newTrackerFixture(t)
	// Fill the cache so names can be resolved for an ID-only response.
	if _, err := tr.Customers(false); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Projects(1, false); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Activities(10, false); err != nil {
		t.Fatal(err)
	}
	f.active[7] = map[string]any{"id": 7, "begin": "2026-10-13T08:00:00+0200", "end": nil, "description": "extern",
		"project":  map[string]any{"id": 10, "name": "Web", "customer": map[string]any{"id": 1, "name": "ACME"}},
		"activity": 101}

	st, err := tr.Sync()
	if err != nil || !st.Running || st.TimesheetID != 7 || st.Description != "extern" {
		t.Fatalf("adopted = %+v, %v", st, err)
	}
	if st.Customer.Name != "ACME" || st.Activity.Name != "Meeting" {
		t.Errorf("names = %+v / %+v", st.Customer, st.Activity)
	}

	delete(f.active, 7) // stopped in the Kimai web UI
	st, err = tr.Sync()
	if err != nil || st.Running || st.Project.ID != 10 {
		t.Fatalf("released = %+v, %v", st, err)
	}
}

func TestTrackerStopTimerDeletedInKimai(t *testing.T) {
	tr, f, _ := newTrackerFixture(t)
	st, err := tr.Start(StartRequest{Customer: Selection{ID: 1}, Project: Selection{ID: 10}, Activity: Selection{ID: 100}})
	if err != nil {
		t.Fatal(err)
	}
	delete(f.active, st.TimesheetID)
	if st, err = tr.Stop(); err != nil || st.Running {
		t.Fatalf("Stop after external delete = %+v, %v", st, err)
	}
}

func TestTrackerListsUseCacheWhenOffline(t *testing.T) {
	tr, f, _ := newTrackerFixture(t)
	if _, err := tr.Customers(false); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Customers(false); err != nil || f.calls["GET /api/customers"] != 1 {
		t.Errorf("fresh cache should avoid a request (calls=%d)", f.calls["GET /api/customers"])
	}

	f.offline = true
	list, err := tr.Customers(true)
	if err != nil || !list.Stale || len(list.Items) != 2 || list.Message == "" {
		t.Fatalf("offline list = %+v, %v", list, err)
	}
	if _, err := tr.Projects(2, false); err == nil {
		t.Error("uncached list while offline should fail")
	}
	if _, err := tr.Start(StartRequest{Customer: Selection{ID: 1}, Project: Selection{ID: 10}, Activity: Selection{ID: 100}}); err == nil {
		t.Error("start while offline should fail")
	}
}

func TestTrackerRequiresSelection(t *testing.T) {
	tr, _, _ := newTrackerFixture(t)
	if _, err := tr.Start(StartRequest{Project: Selection{ID: 10}}); err == nil {
		t.Error("expected validation error")
	}
}

func TestTrackerSelectionSurvivesRestart(t *testing.T) {
	tr, _, _ := newTrackerFixture(t)
	tr.SaveSelection(Selection{ID: 1, Name: "ACME"}, Selection{ID: 10, Name: "Web"}, Selection{}, "Entwurf")
	again := NewTrackerService(tr.conn, tr.settings, NewCatalog(tr.catalog.cachePath, tr.catalog.statePath))
	again.restoreSelection()
	if st := again.State(); st.Project.Name != "Web" || st.Description != "Entwurf" {
		t.Errorf("restored = %+v", st)
	}
}
