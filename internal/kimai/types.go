package kimai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Customer is a Kimai customer.
type Customer struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Number  string `json:"number,omitempty"`
	Color   string `json:"color,omitempty"`
	Visible bool   `json:"visible"`
}

// Project is a Kimai project.
type Project struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Customer Ref    `json:"customer"`
	// ParentTitle is the customer name as delivered by the collection endpoint.
	ParentTitle string `json:"parentTitle,omitempty"`
	Color       string `json:"color,omitempty"`
	Visible     bool   `json:"visible"`
}

// Activity is a Kimai activity. Global activities have no project (Project.ID == 0).
type Activity struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Project Ref    `json:"project"`
	Color   string `json:"color,omitempty"`
	Visible bool   `json:"visible"`
}

// Timesheet is a Kimai timesheet record.
type Timesheet struct {
	ID          int    `json:"id"`
	Begin       Time   `json:"begin"`
	End         *Time  `json:"end"`
	Duration    int    `json:"duration"`
	Description string `json:"description"`
	Project     Ref    `json:"project"`
	Activity    Ref    `json:"activity"`
}

// Running reports whether the timesheet has not been stopped yet.
func (t Timesheet) Running() bool { return t.End == nil || t.End.IsZero() }

// User is the authenticated Kimai user.
type User struct {
	ID          int          `json:"id"`
	Username    string       `json:"username"`
	Alias       string       `json:"alias"`
	Timezone    string       `json:"timezone"`
	Language    string       `json:"language"`
	Preferences []Preference `json:"preferences,omitempty"`
}

// TimeZone returns the user's Kimai time zone (top-level field in Kimai 2,
// falling back to the "timezone" preference used by older versions).
func (u User) TimeZone() string {
	if u.Timezone != "" {
		return u.Timezone
	}
	return u.Preference("timezone")
}

// DisplayName returns the alias if set, otherwise the username.
func (u User) DisplayName() string {
	if strings.TrimSpace(u.Alias) != "" {
		return u.Alias
	}
	return u.Username
}

// Preference returns the value of a user preference, e.g. "timezone".
func (u User) Preference(name string) string {
	for _, p := range u.Preferences {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}

// Preference is a single user preference.
type Preference struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Version describes the Kimai server version.
type Version struct {
	Version   string `json:"version"`
	VersionID int    `json:"versionId"`
	Copyright string `json:"copyright"`
}

// Ref references a related entity. Kimai returns relations either as plain
// IDs (collection endpoints) or as nested objects (expanded endpoints such
// as /api/timesheets/active); Ref accepts both.
type Ref struct {
	ID       int    `json:"id"`
	Name     string `json:"name,omitempty"`
	Customer *Ref   `json:"customer,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (r *Ref) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*r = Ref{}
		return nil
	}
	if len(data) > 0 && data[0] != '{' {
		var id int
		if err := json.Unmarshal(data, &id); err != nil {
			return fmt.Errorf("kimai: invalid reference %s", data)
		}
		*r = Ref{ID: id}
		return nil
	}
	type plain Ref
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*r = Ref(p)
	return nil
}

// Time parses the date formats used by the Kimai API
// (e.g. "2026-10-13T09:00:00+0200").
type Time struct{ time.Time }

var timeLayouts = []string{
	"2006-01-02T15:04:05-0700",
	time.RFC3339,
	"2006-01-02T15:04:05",
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *Time) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil || s == "" {
		t.Time = time.Time{}
		return nil
	}
	for _, layout := range timeLayouts {
		if v, err := time.Parse(layout, s); err == nil {
			t.Time = v
			return nil
		}
	}
	return fmt.Errorf("kimai: cannot parse time %q", s)
}

// MarshalJSON implements json.Marshaler.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Format(time.RFC3339))
}
