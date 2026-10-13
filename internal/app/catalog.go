package app

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
)

// cacheTTL is how long cached lists are served without asking Kimai again.
const cacheTTL = 10 * time.Minute

// maxRecent is the number of recently used entries kept per list.
const maxRecent = 5

// Option is one selectable customer, project or activity.
type Option struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Hint  string `json:"hint,omitempty"`
	Color string `json:"color,omitempty"`
	// Recent marks recently used entries (sorted to the top).
	Recent bool `json:"recent"`
}

// OptionList is returned for each picker.
type OptionList struct {
	Items []Option `json:"items"`
	// Stale is set when Kimai could not be reached and cached data is shown.
	Stale bool `json:"stale"`
	// Message explains why the list is stale.
	Message string `json:"message,omitempty"`
}

// Selection identifies a chosen entry together with its display name.
type Selection struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type cachedList struct {
	FetchedAt time.Time `json:"fetchedAt"`
	Items     []Option  `json:"items"`
}

type cacheFile struct {
	BaseURL    string                `json:"baseUrl"`
	Customers  *cachedList           `json:"customers,omitempty"`
	Projects   map[string]cachedList `json:"projects,omitempty"`
	Activities map[string]cachedList `json:"activities,omitempty"`
}

// Persisted UI state: recently used entries and the last selection.
type stateFile struct {
	BaseURL     string           `json:"baseUrl"`
	Recent      map[string][]int `json:"recent,omitempty"`
	Customer    Selection        `json:"customer"`
	Project     Selection        `json:"project"`
	Activity    Selection        `json:"activity"`
	Description string           `json:"description,omitempty"`
}

// Catalog caches Kimai lists on disk (per Kimai URL) and remembers recently
// used entries.
type Catalog struct {
	cachePath, statePath string
	now                  func() time.Time

	mu    sync.Mutex
	cache cacheFile
	state stateFile
}

// NewCatalog loads cache and state from disk; unreadable files are ignored.
func NewCatalog(cachePath, statePath string) *Catalog {
	c := &Catalog{cachePath: cachePath, statePath: statePath, now: time.Now}
	_ = config.LoadJSON(cachePath, &c.cache)
	_ = config.LoadJSON(statePath, &c.state)
	return c
}

const (
	kindCustomers  = "customers"
	kindProjects   = "projects"
	kindActivities = "activities"
)

// bindTo resets cache and state when the Kimai instance changed.
func (c *Catalog) bindTo(baseURL string) {
	if c.cache.BaseURL != baseURL {
		c.cache = cacheFile{BaseURL: baseURL}
	}
	if c.state.BaseURL != baseURL {
		c.state = stateFile{BaseURL: baseURL}
	}
}

// cached returns a cached list and whether it is still fresh.
func (c *Catalog) cached(baseURL, kind string, parent int) ([]Option, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindTo(baseURL)
	var l *cachedList
	switch kind {
	case kindCustomers:
		l = c.cache.Customers
	case kindProjects:
		if v, ok := c.cache.Projects[strconv.Itoa(parent)]; ok {
			l = &v
		}
	case kindActivities:
		if v, ok := c.cache.Activities[strconv.Itoa(parent)]; ok {
			l = &v
		}
	}
	if l == nil {
		return nil, false, false
	}
	return c.decorate(kind, l.Items), true, c.now().Sub(l.FetchedAt) < cacheTTL
}

// store saves a freshly fetched list.
func (c *Catalog) store(baseURL, kind string, parent int, items []Option) []Option {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindTo(baseURL)
	l := cachedList{FetchedAt: c.now(), Items: items}
	switch kind {
	case kindCustomers:
		c.cache.Customers = &l
	case kindProjects:
		if c.cache.Projects == nil {
			c.cache.Projects = map[string]cachedList{}
		}
		c.cache.Projects[strconv.Itoa(parent)] = l
	case kindActivities:
		if c.cache.Activities == nil {
			c.cache.Activities = map[string]cachedList{}
		}
		c.cache.Activities[strconv.Itoa(parent)] = l
	}
	_ = config.SaveJSON(c.cachePath, c.cache)
	return c.decorate(kind, items)
}

// decorate marks recent entries and sorts them to the top (most recent first),
// the rest alphabetically. Must be called with c.mu held.
func (c *Catalog) decorate(kind string, items []Option) []Option {
	rank := map[int]int{}
	for i, id := range c.state.Recent[kind] {
		rank[id] = i + 1
	}
	out := make([]Option, len(items))
	copy(out, items)
	for i := range out {
		out[i].Recent = rank[out[i].ID] > 0
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank[out[i].ID], rank[out[j].ID]
		switch {
		case ri > 0 && rj > 0:
			return ri < rj
		case ri > 0 || rj > 0:
			return ri > 0
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// markUsed records the selection as most recently used.
func (c *Catalog) markUsed(baseURL string, customer, project, activity Selection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindTo(baseURL)
	if c.state.Recent == nil {
		c.state.Recent = map[string][]int{}
	}
	for kind, id := range map[string]int{kindCustomers: customer.ID, kindProjects: project.ID, kindActivities: activity.ID} {
		if id == 0 {
			continue
		}
		list := []int{id}
		for _, v := range c.state.Recent[kind] {
			if v != id && len(list) < maxRecent {
				list = append(list, v)
			}
		}
		c.state.Recent[kind] = list
	}
	_ = config.SaveJSON(c.statePath, c.state)
}

// saveSelection remembers the current selection and draft description.
func (c *Catalog) saveSelection(baseURL string, customer, project, activity Selection, description string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindTo(baseURL)
	c.state.Customer, c.state.Project, c.state.Activity = customer, project, activity
	c.state.Description = description
	_ = config.SaveJSON(c.statePath, c.state)
}

func (c *Catalog) selection(baseURL string) stateFile {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindTo(baseURL)
	return c.state
}

// name looks up a cached entry name (used when Kimai returns only an ID).
func (c *Catalog) name(kind string, id int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	find := func(items []Option) string {
		for _, o := range items {
			if o.ID == id {
				return o.Name
			}
		}
		return ""
	}
	switch kind {
	case kindCustomers:
		if c.cache.Customers != nil {
			return find(c.cache.Customers.Items)
		}
	case kindProjects:
		for _, l := range c.cache.Projects {
			if n := find(l.Items); n != "" {
				return n
			}
		}
	case kindActivities:
		for _, l := range c.cache.Activities {
			if n := find(l.Items); n != "" {
				return n
			}
		}
	}
	return ""
}
