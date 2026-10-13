// Package kimai implements a client for the Kimai 2.x REST API.
package kimai

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// dateTimeFormat is the "HTML5 datetime-local" format Kimai expects for input.
const dateTimeFormat = "2006-01-02T15:04:05"

// DefaultTimeout bounds every API request.
const DefaultTimeout = 15 * time.Second

// Client talks to one Kimai instance with one API token.
type Client struct {
	baseURL   string
	token     string
	http      *http.Client
	userAgent string
	// Location is the time zone used to format timestamps sent to Kimai.
	// Kimai interprets them in the user's configured time zone; set it from
	// the "timezone" user preference. Defaults to time.Local.
	Location *time.Location
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// New creates a client. baseURL is normalised with NormalizeBaseURL.
func New(baseURL, token string, opts ...Option) (*Client, error) {
	u, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("%w: missing API token", ErrUnauthorized)
	}
	c := &Client{
		baseURL:   u,
		token:     strings.TrimSpace(token),
		http:      &http.Client{Timeout: DefaultTimeout},
		userAgent: "kimai-tracker",
		Location:  time.Local,
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// BaseURL returns the normalised base URL.
func (c *Client) BaseURL() string { return c.baseURL }

// NormalizeBaseURL trims whitespace and trailing slashes, adds a scheme when
// none is given (http:// for localhost, https:// otherwise) and strips a
// trailing "/api" so users can paste either form.
func NormalizeBaseURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidURL)
	}
	if !strings.Contains(s, "://") {
		if isLoopback(s) {
			s = "http://" + s
		} else {
			s = "https://" + s
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%w: unsupported scheme %q", ErrInvalidURL, u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimRight(u.Path, "/")
	u.Path = strings.TrimSuffix(u.Path, "/api")
	u.RawPath = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func isLoopback(hostAndPath string) bool {
	host := hostAndPath
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if h, _, ok := strings.Cut(host, "]"); ok { // [::1]:8001
		host = strings.TrimPrefix(h, "[")
	} else if h, _, ok := strings.Cut(host, ":"); ok && strings.Count(host, ":") == 1 {
		host = h
	}
	return host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1"
}

// ---- Endpoints -------------------------------------------------------------

// Version returns the server version (GET /api/version).
func (c *Client) Version(ctx context.Context) (Version, error) {
	var v Version
	err := c.do(ctx, http.MethodGet, "/api/version", nil, nil, &v)
	return v, err
}

// Me returns the authenticated user (GET /api/users/me).
func (c *Client) Me(ctx context.Context) (User, error) {
	var u User
	err := c.do(ctx, http.MethodGet, "/api/users/me", nil, nil, &u)
	return u, err
}

// Customers lists visible customers (GET /api/customers).
func (c *Client) Customers(ctx context.Context) ([]Customer, error) {
	var out []Customer
	err := c.do(ctx, http.MethodGet, "/api/customers", visibleQuery(), nil, &out)
	return out, err
}

// Projects lists visible projects of a customer (GET /api/projects?customer=ID).
func (c *Client) Projects(ctx context.Context, customerID int) ([]Project, error) {
	q := visibleQuery()
	q.Set("customer", strconv.Itoa(customerID))
	var out []Project
	err := c.do(ctx, http.MethodGet, "/api/projects", q, nil, &out)
	return out, err
}

// Activities lists visible activities usable for a project, including global
// activities (GET /api/activities?project=ID).
func (c *Client) Activities(ctx context.Context, projectID int) ([]Activity, error) {
	q := visibleQuery()
	q.Set("project", strconv.Itoa(projectID))
	var out []Activity
	err := c.do(ctx, http.MethodGet, "/api/activities", q, nil, &out)
	return out, err
}

// ActiveTimesheets returns the user's running timesheets (GET /api/timesheets/active).
func (c *Client) ActiveTimesheets(ctx context.Context) ([]Timesheet, error) {
	var out []Timesheet
	err := c.do(ctx, http.MethodGet, "/api/timesheets/active", nil, nil, &out)
	return out, err
}

// StartInput describes a new running timesheet.
type StartInput struct {
	ProjectID   int
	ActivityID  int
	Description string
	// Begin defaults to now.
	Begin time.Time
}

// StartTimesheet creates a running timesheet (POST /api/timesheets).
func (c *Client) StartTimesheet(ctx context.Context, in StartInput) (Timesheet, error) {
	begin := in.Begin
	if begin.IsZero() {
		begin = time.Now()
	}
	body := map[string]any{
		"project":     in.ProjectID,
		"activity":    in.ActivityID,
		"begin":       begin.In(c.location()).Format(dateTimeFormat),
		"description": in.Description,
	}
	var ts Timesheet
	err := c.do(ctx, http.MethodPost, "/api/timesheets", nil, body, &ts)
	return ts, err
}

// StopTimesheet stops a running timesheet (PATCH /api/timesheets/{id}/stop).
func (c *Client) StopTimesheet(ctx context.Context, id int) (Timesheet, error) {
	var ts Timesheet
	err := c.do(ctx, http.MethodPatch, fmt.Sprintf("/api/timesheets/%d/stop", id), nil, nil, &ts)
	return ts, err
}

// UpdateDescription sets the description of a timesheet (PATCH /api/timesheets/{id}).
func (c *Client) UpdateDescription(ctx context.Context, id int, description string) (Timesheet, error) {
	var ts Timesheet
	body := map[string]any{"description": description}
	err := c.do(ctx, http.MethodPatch, fmt.Sprintf("/api/timesheets/%d", id), nil, body, &ts)
	return ts, err
}

// Timesheets returns the user's timesheets that begin in [from, to], oldest
// first, with project, customer and activity expanded (GET /api/timesheets).
// A running timesheet is included.
func (c *Client) Timesheets(ctx context.Context, from, to time.Time) ([]Timesheet, error) {
	loc := c.location()
	q := url.Values{
		"begin":   {from.In(loc).Format(dateTimeFormat)},
		"end":     {to.In(loc).Format(dateTimeFormat)},
		"full":    {"true"},
		"orderBy": {"begin"},
		"order":   {"ASC"},
		"size":    {"500"},
	}
	var out []Timesheet
	err := c.do(ctx, http.MethodGet, "/api/timesheets", q, nil, &out)
	return out, err
}

// EntryInput describes a timesheet that is created or changed afterwards.
type EntryInput struct {
	ProjectID   int
	ActivityID  int
	Description string
	Begin       time.Time
	// End is required for new entries; on updates a zero End leaves a
	// running timesheet running.
	End time.Time
}

func (c *Client) entryBody(in EntryInput) map[string]any {
	body := map[string]any{
		"project":     in.ProjectID,
		"activity":    in.ActivityID,
		"begin":       in.Begin.In(c.location()).Format(dateTimeFormat),
		"description": in.Description,
	}
	if !in.End.IsZero() {
		body["end"] = in.End.In(c.location()).Format(dateTimeFormat)
	}
	return body
}

// CreateTimesheet adds a finished timesheet (POST /api/timesheets).
func (c *Client) CreateTimesheet(ctx context.Context, in EntryInput) (Timesheet, error) {
	if in.End.IsZero() {
		return Timesheet{}, errors.New("kimai: end time required")
	}
	var ts Timesheet
	err := c.do(ctx, http.MethodPost, "/api/timesheets", url.Values{"full": {"true"}}, c.entryBody(in), &ts)
	return ts, err
}

// UpdateTimesheet changes times, project, activity and description of a
// timesheet (PATCH /api/timesheets/{id}).
func (c *Client) UpdateTimesheet(ctx context.Context, id int, in EntryInput) (Timesheet, error) {
	var ts Timesheet
	err := c.do(ctx, http.MethodPatch, fmt.Sprintf("/api/timesheets/%d", id), url.Values{"full": {"true"}}, c.entryBody(in), &ts)
	return ts, err
}

// DeleteTimesheet deletes a timesheet (DELETE /api/timesheets/{id}).
func (c *Client) DeleteTimesheet(ctx context.Context, id int) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/timesheets/%d", id), nil, nil, nil)
}

// ---- Transport -------------------------------------------------------------

func visibleQuery() url.Values {
	return url.Values{"visible": {"1"}, "orderBy": {"name"}, "order": {"ASC"}}
}

func (c *Client) location() *time.Location {
	if c.Location != nil {
		return c.Location
	}
	return time.Local
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return classifyTransportError(err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return &NetworkError{Err: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		var payload struct {
			Message string          `json:"message"`
			Errors  json.RawMessage `json:"errors"`
		}
		if isJSON(resp.Header.Get("Content-Type")) && json.Unmarshal(data, &payload) == nil {
			apiErr.Message = payload.Message
			apiErr.Details = validationMessages(payload.Errors)
		}
		return apiErr
	}

	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if !isJSON(resp.Header.Get("Content-Type")) {
		return fmt.Errorf("%w: content type %q", ErrUnexpectedResponse, resp.Header.Get("Content-Type"))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.Join(ErrUnexpectedResponse, err)
	}
	return nil
}

func classifyTransportError(err error) error {
	var certErr *tls.CertificateVerificationError
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalidErr x509.CertificateInvalidError
	switch {
	case errors.As(err, &certErr), errors.As(err, &unknownAuth), errors.As(err, &hostErr), errors.As(err, &invalidErr):
		return fmt.Errorf("%w: %v", ErrCertificate, err)
	case strings.Contains(err.Error(), "server gave HTTP response to HTTPS client"):
		return fmt.Errorf("%w: %v", ErrNotHTTPS, err)
	}
	return &NetworkError{Err: err}
}

func isJSON(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && (mt == "application/json" || strings.HasSuffix(mt, "+json"))
}

// ConnectionInfo is the result of a successful connection check.
type ConnectionInfo struct {
	Version Version
	User    User
}

// Check verifies URL and token by fetching the server version and the current
// user. Use it for "test connection".
func (c *Client) Check(ctx context.Context) (ConnectionInfo, error) {
	var info ConnectionInfo
	user, err := c.Me(ctx)
	if err != nil {
		return info, err
	}
	info.User = user
	// The version endpoint is informational; older or locked-down servers may refuse it.
	if v, err := c.Version(ctx); err == nil {
		info.Version = v
	} else if errors.Is(err, ErrOffline) {
		return info, err
	}
	return info, nil
}

// validationMessages collects the messages of a Kimai/Symfony form error tree:
// {"errors":["…"],"children":{"end":{"errors":["…"]}}}.
func validationMessages(raw json.RawMessage) []string {
	var node struct {
		Errors   []string                   `json:"errors"`
		Children map[string]json.RawMessage `json:"children"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &node) != nil {
		return nil
	}
	out := node.Errors
	names := make([]string, 0, len(node.Children))
	for name := range node.Children {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, validationMessages(node.Children[name])...)
	}
	return out
}
