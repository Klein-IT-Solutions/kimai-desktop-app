package app

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestCatalog(t *testing.T) *Catalog {
	dir := t.TempDir()
	return NewCatalog(filepath.Join(dir, "cache.json"), filepath.Join(dir, "state.json"))
}

func names(items []Option) []string {
	var out []string
	for _, o := range items {
		out = append(out, o.Name)
	}
	return out
}

func TestCatalogSortsRecentFirst(t *testing.T) {
	c := newTestCatalog(t)
	const url = "https://k"
	items := []Option{{ID: 1, Name: "beta"}, {ID: 2, Name: "Alpha"}, {ID: 3, Name: "Gamma"}, {ID: 4, Name: "delta"}}
	got := c.store(url, kindCustomers, 0, items)
	if want := "Alpha beta delta Gamma"; joined(names(got)) != want {
		t.Fatalf("alphabetical = %v", names(got))
	}

	c.markUsed(url, Selection{ID: 3}, Selection{}, Selection{})
	c.markUsed(url, Selection{ID: 4}, Selection{}, Selection{})
	got, ok, fresh := c.cached(url, kindCustomers, 0)
	if !ok || !fresh {
		t.Fatal("expected fresh cache")
	}
	if want := "delta Gamma Alpha beta"; joined(names(got)) != want {
		t.Errorf("recent first = %v", names(got))
	}
	if !got[0].Recent || !got[1].Recent || got[2].Recent {
		t.Errorf("recent flags = %+v", got)
	}
}

func TestCatalogRecentLimitAndDedup(t *testing.T) {
	c := newTestCatalog(t)
	for i := 1; i <= 7; i++ {
		c.markUsed("u", Selection{ID: i}, Selection{}, Selection{})
	}
	c.markUsed("u", Selection{ID: 5}, Selection{}, Selection{})
	got := c.state.Recent[kindCustomers]
	want := []int{5, 7, 6, 4, 3}
	if len(got) != len(want) {
		t.Fatalf("recent = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("recent = %v, want %v", got, want)
		}
	}
}

func TestCatalogExpiryPersistenceAndInstanceSwitch(t *testing.T) {
	dir := t.TempDir()
	c := NewCatalog(filepath.Join(dir, "cache.json"), filepath.Join(dir, "state.json"))
	now := time.Now()
	c.now = func() time.Time { return now }
	c.store("https://a", kindProjects, 7, []Option{{ID: 70, Name: "P"}})
	c.saveSelection("https://a", Selection{ID: 7, Name: "C"}, Selection{ID: 70, Name: "P"}, Selection{}, "draft")

	reloaded := NewCatalog(filepath.Join(dir, "cache.json"), filepath.Join(dir, "state.json"))
	reloaded.now = func() time.Time { return now.Add(cacheTTL + time.Second) }
	items, ok, fresh := reloaded.cached("https://a", kindProjects, 7)
	if !ok || fresh || len(items) != 1 {
		t.Fatalf("expected stale cached entry, got ok=%v fresh=%v %v", ok, fresh, items)
	}
	if s := reloaded.selection("https://a"); s.Project.Name != "P" || s.Description != "draft" {
		t.Errorf("selection = %+v", s)
	}
	if reloaded.name(kindProjects, 70) != "P" {
		t.Error("name lookup failed")
	}

	// Another Kimai instance must not see the first one's data.
	if _, ok, _ := reloaded.cached("https://b", kindProjects, 7); ok {
		t.Error("cache leaked across instances")
	}
	if s := reloaded.selection("https://b"); s.Project.ID != 0 {
		t.Errorf("selection leaked across instances: %+v", s)
	}
}

func joined(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += " "
		}
		out += v
	}
	return out
}
