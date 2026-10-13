//go:build integration

// Integration tests against a real Kimai instance. They create and delete
// timesheets, so use a test instance:
//
//	make kimai-up        # local Kimai in Docker, seeded with test data
//	KIMAI_TEST_URL=http://localhost:8001 KIMAI_TEST_TOKEN=... make test-integration
package kimai

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

func integrationClient(t *testing.T) *Client {
	t.Helper()
	url, token := os.Getenv("KIMAI_TEST_URL"), os.Getenv("KIMAI_TEST_TOKEN")
	if url == "" || token == "" {
		t.Skip("KIMAI_TEST_URL and KIMAI_TEST_TOKEN not set")
	}
	c, err := New(url, token, WithHTTPClient(&http.Client{Timeout: DefaultTimeout, Transport: &http.Transport{Proxy: nil}}))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestIntegrationFullFlow(t *testing.T) {
	c := integrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	info, err := c.Check(ctx)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	t.Logf("connected to Kimai %s as %s (time zone %q)", info.Version.Version, info.User.DisplayName(), info.User.TimeZone())
	if info.Version.Version == "" || info.User.Username == "" {
		t.Errorf("incomplete connection info: %+v", info)
	}
	if tz := info.User.TimeZone(); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			t.Fatalf("load time zone %q: %v", tz, err)
		}
		c.Location = loc
	}

	// Clean slate: stop timers left over from earlier runs.
	active, err := c.ActiveTimesheets(ctx)
	if err != nil {
		t.Fatalf("ActiveTimesheets: %v", err)
	}
	for _, ts := range active {
		if _, err := c.StopTimesheet(ctx, ts.ID); err != nil {
			t.Fatalf("stop leftover %d: %v", ts.ID, err)
		}
	}

	customers, err := c.Customers(ctx)
	if err != nil || len(customers) == 0 {
		t.Fatalf("Customers: %v (%d)", err, len(customers))
	}
	var projects []Project
	var customer Customer
	for _, cu := range customers {
		if projects, err = c.Projects(ctx, cu.ID); err != nil {
			t.Fatalf("Projects(%d): %v", cu.ID, err)
		}
		for _, p := range projects {
			if p.Customer.ID != cu.ID {
				t.Errorf("project %q has customer %d, want %d", p.Name, p.Customer.ID, cu.ID)
			}
		}
		if len(projects) > 0 {
			customer = cu
			break
		}
	}
	if len(projects) == 0 {
		t.Fatal("no customer with projects – seed the instance first (scripts/kimai-seed.sh)")
	}
	project := projects[0]

	activities, err := c.Activities(ctx, project.ID)
	if err != nil || len(activities) == 0 {
		t.Fatalf("Activities(%d): %v (%d)", project.ID, err, len(activities))
	}
	var globals int
	for _, a := range activities {
		if a.Project.ID == 0 {
			globals++
		} else if a.Project.ID != project.ID {
			t.Errorf("activity %q belongs to project %d, want %d or global", a.Name, a.Project.ID, project.ID)
		}
	}
	t.Logf("customer %q → project %q → %d activities (%d global)", customer.Name, project.Name, len(activities), globals)
	activity := activities[0]

	// Start
	begin := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	started, err := c.StartTimesheet(ctx, StartInput{
		ProjectID: project.ID, ActivityID: activity.ID, Description: "kimai-tracker integration test", Begin: begin,
	})
	if err != nil {
		t.Fatalf("StartTimesheet: %v", err)
	}
	t.Cleanup(func() { deleteTimesheet(t, c, started.ID) })
	if !started.Running() || started.Project.ID != project.ID || started.Activity.ID != activity.ID {
		t.Errorf("started = %+v", started)
	}
	if !started.Begin.Equal(begin) {
		t.Errorf("begin = %v, want %v – time zone handling is off", started.Begin.Time, begin)
	}

	// Active timer is visible with expanded relations.
	active, err = c.ActiveTimesheets(ctx)
	if err != nil || len(active) != 1 || active[0].ID != started.ID {
		t.Fatalf("ActiveTimesheets = %+v, %v", active, err)
	}
	if active[0].Project.Name != project.Name || active[0].Activity.Name != activity.Name {
		t.Errorf("active relations = %+v / %+v", active[0].Project, active[0].Activity)
	}
	if active[0].Project.Customer == nil || active[0].Project.Customer.ID != customer.ID {
		t.Errorf("active customer = %+v", active[0].Project.Customer)
	}

	// Description
	updated, err := c.UpdateDescription(ctx, started.ID, "geändert ✓")
	if err != nil || updated.Description != "geändert ✓" {
		t.Fatalf("UpdateDescription = %+v, %v", updated, err)
	}

	// Stop
	stopped, err := c.StopTimesheet(ctx, started.ID)
	if err != nil || stopped.Running() {
		t.Fatalf("StopTimesheet = %+v, %v", stopped, err)
	}
	if stopped.Duration < 290 {
		t.Errorf("duration = %ds, want ≥ 5 minutes", stopped.Duration)
	}
	if active, _ = c.ActiveTimesheets(ctx); len(active) != 0 {
		t.Errorf("still active after stop: %+v", active)
	}

	// Entries added afterwards: yesterday 08:00–09:30, then moved and deleted.
	loc := c.location()
	y := time.Now().In(loc).AddDate(0, 0, -1)
	day := time.Date(y.Year(), y.Month(), y.Day(), 0, 0, 0, 0, loc)
	at := func(h, m int) time.Time { return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }
	added, err := c.CreateTimesheet(ctx, EntryInput{
		ProjectID: project.ID, ActivityID: activity.ID, Description: "nachgetragen", Begin: at(8, 0), End: at(9, 30),
	})
	if err != nil {
		t.Fatalf("CreateTimesheet: %v", err)
	}
	t.Cleanup(func() { deleteTimesheet(t, c, added.ID) })
	if added.Running() || added.Duration != 5400 || !added.Begin.Equal(at(8, 0)) {
		t.Errorf("added = %+v", added)
	}
	list, err := c.Timesheets(ctx, day, day.AddDate(0, 0, 1).Add(-time.Second))
	if err != nil {
		t.Fatalf("Timesheets: %v", err)
	}
	var found *Timesheet
	for i := range list {
		if list[i].ID == added.ID {
			found = &list[i]
		}
	}
	if found == nil || found.Project.Name != project.Name || found.Project.Customer == nil || found.Activity.Name != activity.Name {
		t.Fatalf("added entry not listed with relations: %+v", list)
	}
	moved, err := c.UpdateTimesheet(ctx, added.ID, EntryInput{
		ProjectID: project.ID, ActivityID: activity.ID, Description: "verschoben", Begin: at(10, 0), End: at(10, 45),
	})
	if err != nil || moved.Duration != 2700 || moved.Description != "verschoben" {
		t.Fatalf("UpdateTimesheet = %+v, %v", moved, err)
	}
	_, err = c.CreateTimesheet(ctx, EntryInput{ProjectID: project.ID, ActivityID: activity.ID, Begin: at(12, 0), End: at(11, 0)})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || len(apiErr.Details) == 0 {
		t.Errorf("end before begin: err = %v, want validation details", err)
	}
	if err := c.DeleteTimesheet(ctx, added.ID); err != nil {
		t.Fatalf("DeleteTimesheet: %v", err)
	}
}

func TestIntegrationErrors(t *testing.T) {
	c := integrationClient(t)
	ctx := context.Background()

	bad, _ := New(c.BaseURL(), "definitely-not-a-valid-token", WithHTTPClient(c.http))
	if _, err := bad.Me(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("bad token: err = %v, want ErrUnauthorized", err)
	}
	if _, err := c.StopTimesheet(ctx, 999999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown timesheet: err = %v, want ErrNotFound", err)
	}
	wrong, _ := New(c.BaseURL()+"/does-not-exist", "x", WithHTTPClient(c.http))
	if _, err := wrong.Me(ctx); err == nil {
		t.Error("wrong base URL: expected an error")
	} else {
		t.Logf("wrong base URL → %v", err)
	}
}

func deleteTimesheet(t *testing.T, c *Client, id int) {
	t.Helper()
	if err := c.do(context.Background(), http.MethodDelete, "/api/timesheets/"+strconv.Itoa(id), nil, nil, nil); err != nil {
		t.Logf("cleanup: delete timesheet %d: %v", id, err)
	}
}
