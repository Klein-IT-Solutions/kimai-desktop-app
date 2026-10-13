package kimai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testToken = "test-token"

// fakeKimai serves canned responses and records the last request.
type fakeKimai struct {
	t        *testing.T
	routes   map[string]func(w http.ResponseWriter, r *http.Request)
	lastBody map[string]any
	lastReq  *http.Request
}

func newFake(t *testing.T) (*fakeKimai, *Client) {
	t.Helper()
	f := &fakeKimai{t: t, routes: map[string]func(http.ResponseWriter, *http.Request){}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.lastReq = r
		f.lastBody = nil
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			if err := json.Unmarshal(b, &f.lastBody); err != nil {
				t.Errorf("request body is not JSON: %s", b)
			}
		}
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			writeJSON(w, http.StatusUnauthorized, `{"code":401,"message":"Invalid credentials"}`)
			return
		}
		h, ok := f.routes[r.Method+" "+r.URL.Path]
		if !ok {
			writeJSON(w, http.StatusNotFound, `{"code":404,"message":"Not found"}`)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL+"/", testToken)
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}

func (f *fakeKimai) handle(route string, status int, body string) {
	f.routes[route] = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, status, body) }
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://kimai.example.com":           "https://kimai.example.com",
		"  https://kimai.example.com/  ":      "https://kimai.example.com",
		"kimai.example.com":                   "https://kimai.example.com",
		"https://kimai.example.com/api":       "https://kimai.example.com",
		"https://kimai.example.com/api/":      "https://kimai.example.com",
		"https://example.com/kimai/":          "https://example.com/kimai",
		"http://localhost:8001":               "http://localhost:8001",
		"localhost:8001/api/":                 "http://localhost:8001",
		"127.0.0.1:8001":                      "http://127.0.0.1:8001",
		"[::1]:8001":                          "http://[::1]:8001",
		"localhost.example.com":               "https://localhost.example.com",
		"https://kimai.example.com/?x=1#frag": "https://kimai.example.com",
	}
	for in, want := range cases {
		got, err := NormalizeBaseURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "ftp://kimai.example.com", "https://"} {
		if _, err := NormalizeBaseURL(bad); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("NormalizeBaseURL(%q) error = %v, want ErrInvalidURL", bad, err)
		}
	}
}

func TestNewRequiresToken(t *testing.T) {
	if _, err := New("https://kimai.example.com", " "); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestCheck(t *testing.T) {
	f, c := newFake(t)
	// Shape as returned by Kimai 2.69: time zone is a top-level field, preference values may be null.
	f.handle("GET /api/users/me", 200, `{"id":3,"username":"henning","alias":"Henning K.","timezone":"Europe/Berlin","language":"de","preferences":[{"name":"holidays","value":null},{"name":"work_monday","value":"0"}]}`)
	f.handle("GET /api/version", 200, `{"version":"2.40.0","versionId":24000,"copyright":"Kimai"}`)

	info, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.User.DisplayName() != "Henning K." || info.User.TimeZone() != "Europe/Berlin" || info.User.Preference("work_monday") != "0" {
		t.Errorf("user = %+v", info.User)
	}
	if info.Version.Version != "2.40.0" {
		t.Errorf("version = %+v", info.Version)
	}
	if got := f.lastReq.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
}

func TestCheckToleratesMissingVersionEndpoint(t *testing.T) {
	f, c := newFake(t)
	f.handle("GET /api/users/me", 200, `{"id":3,"username":"henning","alias":null,"preferences":[{"name":"timezone","value":"Europe/Vienna"}]}`)
	info, err := c.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.User.DisplayName() != "henning" {
		t.Errorf("DisplayName = %q", info.User.DisplayName())
	}
	if info.User.TimeZone() != "Europe/Vienna" {
		t.Errorf("TimeZone fallback = %q", info.User.TimeZone())
	}
}

func TestUnauthorized(t *testing.T) {
	_, c := newFake(t)
	bad, _ := New(c.BaseURL(), "wrong")
	_, err := bad.Me(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "Invalid credentials" {
		t.Errorf("APIError = %+v", apiErr)
	}
}

func TestForbiddenAndNotFound(t *testing.T) {
	f, c := newFake(t)
	f.handle("GET /api/customers", 403, `{"code":403,"message":"Access denied."}`)
	if _, err := c.Customers(context.Background()); !errors.Is(err, ErrForbidden) {
		t.Errorf("Customers err = %v, want ErrForbidden", err)
	}
	if _, err := c.StopTimesheet(context.Background(), 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("Stop err = %v, want ErrNotFound", err)
	}
}

func TestUnexpectedHTMLResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html>Login</html>")
	}))
	defer srv.Close()
	c, _ := New(srv.URL, testToken)
	if _, err := c.Me(context.Background()); !errors.Is(err, ErrUnexpectedResponse) {
		t.Fatalf("err = %v, want ErrUnexpectedResponse", err)
	}
}

func TestOffline(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c, _ := New(url, testToken)
	_, err := c.Me(context.Background())
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("err = %v, want ErrOffline", err)
	}
}

func TestContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	c, _ := New(srv.URL, testToken)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Me(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}

func TestCustomersProjectsActivities(t *testing.T) {
	f, c := newFake(t)
	f.handle("GET /api/customers", 200, `[{"id":1,"name":"ACME","number":"C-1","color":"#ff0000","visible":true}]`)
	f.handle("GET /api/projects", 200, `[{"id":10,"name":"Website","customer":1,"parentTitle":"ACME","visible":true}]`)
	f.handle("GET /api/activities", 200, `[{"id":100,"name":"Dev","project":10,"visible":true},{"id":101,"name":"Meeting","project":null,"visible":true}]`)
	ctx := context.Background()

	customers, err := c.Customers(ctx)
	if err != nil || len(customers) != 1 || customers[0].Name != "ACME" {
		t.Fatalf("customers = %+v, %v", customers, err)
	}
	if q := f.lastReq.URL.Query(); q.Get("visible") != "1" || q.Get("orderBy") != "name" {
		t.Errorf("customer query = %v", q)
	}

	projects, err := c.Projects(ctx, 1)
	if err != nil || len(projects) != 1 || projects[0].Customer.ID != 1 || projects[0].ParentTitle != "ACME" {
		t.Fatalf("projects = %+v, %v", projects, err)
	}
	if got := f.lastReq.URL.Query().Get("customer"); got != "1" {
		t.Errorf("customer param = %q", got)
	}

	activities, err := c.Activities(ctx, 10)
	if err != nil || len(activities) != 2 {
		t.Fatalf("activities = %+v, %v", activities, err)
	}
	if got := f.lastReq.URL.Query().Get("project"); got != "10" {
		t.Errorf("project param = %q", got)
	}
	if activities[0].Project.ID != 10 || activities[1].Project.ID != 0 {
		t.Errorf("activity projects = %+v / %+v", activities[0].Project, activities[1].Project)
	}
}

func TestActiveTimesheetsExpanded(t *testing.T) {
	f, c := newFake(t)
	f.handle("GET /api/timesheets/active", 200, `[{
		"id": 42,
		"begin": "2026-10-13T09:15:00+0200",
		"end": null,
		"duration": 0,
		"description": "Sprint planning",
		"activity": {"id": 100, "name": "Dev"},
		"project": {"id": 10, "name": "Website", "customer": {"id": 1, "name": "ACME"}}
	}]`)

	list, err := c.ActiveTimesheets(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("active = %+v, %v", list, err)
	}
	ts := list[0]
	if !ts.Running() || ts.Description != "Sprint planning" {
		t.Errorf("timesheet = %+v", ts)
	}
	if ts.Project.Name != "Website" || ts.Project.Customer == nil || ts.Project.Customer.Name != "ACME" || ts.Activity.Name != "Dev" {
		t.Errorf("relations = %+v / %+v", ts.Project, ts.Activity)
	}
	want := time.Date(2026, 10, 13, 7, 15, 0, 0, time.UTC)
	if !ts.Begin.Equal(want) {
		t.Errorf("begin = %v, want %v", ts.Begin, want)
	}
}

func TestStartTimesheet(t *testing.T) {
	f, c := newFake(t)
	f.handle("POST /api/timesheets", 200, `{"id":43,"begin":"2026-10-13T10:00:00+0200","end":null,"project":10,"activity":100,"description":"Hi"}`)
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("tzdata not available")
	}
	c.Location = berlin

	ts, err := c.StartTimesheet(context.Background(), StartInput{
		ProjectID: 10, ActivityID: 100, Description: "Hi",
		Begin: time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ts.ID != 43 || ts.Project.ID != 10 || !ts.Running() {
		t.Errorf("timesheet = %+v", ts)
	}
	b := f.lastBody
	if b["project"] != float64(10) || b["activity"] != float64(100) || b["description"] != "Hi" {
		t.Errorf("body = %v", b)
	}
	if b["begin"] != "2026-10-13T10:00:00" {
		t.Errorf("begin = %v, want local Kimai time 2026-10-13T10:00:00", b["begin"])
	}
	if ct := f.lastReq.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestStopAndUpdateDescription(t *testing.T) {
	f, c := newFake(t)
	f.handle("PATCH /api/timesheets/43/stop", 200, `{"id":43,"begin":"2026-10-13T10:00:00+0200","end":"2026-10-13T11:30:00+0200","duration":5400}`)
	f.handle("PATCH /api/timesheets/43", 200, `{"id":43,"description":"Updated"}`)
	ctx := context.Background()

	ts, err := c.StopTimesheet(ctx, 43)
	if err != nil || ts.Running() || ts.Duration != 5400 {
		t.Fatalf("stop = %+v, %v", ts, err)
	}

	ts, err = c.UpdateDescription(ctx, 43, "Updated")
	if err != nil || ts.Description != "Updated" {
		t.Fatalf("update = %+v, %v", ts, err)
	}
	if f.lastBody["description"] != "Updated" || len(f.lastBody) != 1 {
		t.Errorf("body = %v", f.lastBody)
	}
}

func TestRefUnmarshal(t *testing.T) {
	var r struct{ A, B, C Ref }
	if err := json.Unmarshal([]byte(`{"A":5,"B":null,"C":{"id":7,"name":"X"}}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.A.ID != 5 || r.B.ID != 0 || r.C.ID != 7 || r.C.Name != "X" {
		t.Errorf("refs = %+v", r)
	}
	if err := json.Unmarshal([]byte(`{"A":"nope"}`), &r); err == nil || !strings.Contains(err.Error(), "invalid reference") {
		t.Errorf("err = %v", err)
	}
}

func TestHTTPSAgainstPlainHTTPServer(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	c, _ := New(strings.Replace(srv.URL, "http://", "https://", 1), testToken)
	_, err := c.Me(context.Background())
	if !errors.Is(err, ErrNotHTTPS) {
		t.Fatalf("err = %v, want ErrNotHTTPS", err)
	}
}

func TestUntrustedCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler()) // self-signed
	defer srv.Close()
	c, _ := New(srv.URL, testToken)
	_, err := c.Me(context.Background())
	if !errors.Is(err, ErrCertificate) {
		t.Fatalf("err = %v, want ErrCertificate", err)
	}
	if errors.Is(err, ErrOffline) {
		t.Error("certificate errors must not be reported as offline")
	}
}

func TestTimesheetsOfDay(t *testing.T) {
	f, c := newFake(t)
	f.handle("GET /api/timesheets", 200, `[
		{"id":1,"begin":"2026-10-13T08:00:00+0200","end":"2026-10-13T09:30:00+0200","duration":5400,
		 "project":{"id":10,"name":"Website","customer":{"id":1,"name":"ACME"}},"activity":{"id":100,"name":"Dev"}},
		{"id":2,"begin":"2026-10-13T10:00:00+0200","end":null,"project":{"id":11,"name":"App","customer":{"id":1,"name":"ACME"}},"activity":{"id":101,"name":"Meeting"}}
	]`)
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("tzdata not available")
	}
	c.Location = berlin
	day := time.Date(2026, 10, 13, 0, 0, 0, 0, berlin)
	list, err := c.Timesheets(context.Background(), day, day.Add(24*time.Hour-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Project.Customer.Name != "ACME" || list[0].Activity.Name != "Dev" || !list[1].Running() {
		t.Errorf("list = %+v", list)
	}
	q := f.lastReq.URL.Query()
	if q.Get("begin") != "2026-10-13T00:00:00" || q.Get("end") != "2026-10-13T23:59:59" || q.Get("full") != "true" || q.Get("order") != "ASC" {
		t.Errorf("query = %v", q)
	}
}

func TestCreateUpdateDeleteTimesheet(t *testing.T) {
	f, c := newFake(t)
	f.handle("POST /api/timesheets", 200, `{"id":7,"begin":"2026-10-13T08:00:00+0200","end":"2026-10-13T09:00:00+0200","duration":3600,"project":10,"activity":100}`)
	f.handle("PATCH /api/timesheets/7", 200, `{"id":7,"begin":"2026-10-13T08:15:00+0200","end":null,"project":10,"activity":100}`)
	f.handle("DELETE /api/timesheets/7", 204, ``)
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("tzdata not available")
	}
	c.Location = berlin
	ctx := context.Background()
	begin := time.Date(2026, 10, 13, 8, 0, 0, 0, berlin)

	if _, err := c.CreateTimesheet(ctx, EntryInput{ProjectID: 10, ActivityID: 100, Begin: begin}); err == nil {
		t.Error("create without end should fail")
	}
	ts, err := c.CreateTimesheet(ctx, EntryInput{ProjectID: 10, ActivityID: 100, Description: "x", Begin: begin, End: begin.Add(time.Hour)})
	if err != nil || ts.ID != 7 || ts.Running() {
		t.Fatalf("create = %+v, %v", ts, err)
	}
	if b := f.lastBody; b["begin"] != "2026-10-13T08:00:00" || b["end"] != "2026-10-13T09:00:00" || b["description"] != "x" {
		t.Errorf("create body = %v", b)
	}

	// Updating a running entry must not send an end time.
	if _, err := c.UpdateTimesheet(ctx, 7, EntryInput{ProjectID: 10, ActivityID: 100, Begin: begin.Add(15 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.lastBody["end"]; ok || f.lastBody["begin"] != "2026-10-13T08:15:00" {
		t.Errorf("update body = %v", f.lastBody)
	}

	if err := c.DeleteTimesheet(ctx, 7); err != nil {
		t.Fatal(err)
	}
}

func TestValidationErrorDetails(t *testing.T) {
	f, c := newFake(t)
	f.handle("POST /api/timesheets", 400, `{"code":400,"message":"Validation Failed","errors":{"errors":["The end date must not be earlier than the start date."],"children":{"begin":{},"project":{"errors":["This value is not valid."]}}}}`)
	now := time.Now()
	_, err := c.CreateTimesheet(context.Background(), EntryInput{ProjectID: 1, ActivityID: 1, Begin: now, End: now.Add(-time.Hour)})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || len(apiErr.Details) != 2 || apiErr.Details[1] != "This value is not valid." {
		t.Fatalf("err = %#v", err)
	}
}
