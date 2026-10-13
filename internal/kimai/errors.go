package kimai

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Sentinel errors; test with errors.Is.
var (
	// ErrUnauthorized means the API token is missing, invalid or revoked (HTTP 401).
	ErrUnauthorized = errors.New("kimai: unauthorized")
	// ErrForbidden means the user lacks the permission for the request (HTTP 403).
	ErrForbidden = errors.New("kimai: forbidden")
	// ErrNotFound means the resource or API endpoint does not exist (HTTP 404).
	ErrNotFound = errors.New("kimai: not found")
	// ErrOffline means the server could not be reached (DNS, connection, timeout, TLS).
	ErrOffline = errors.New("kimai: server unreachable")
	// ErrUnexpectedResponse means the server did not answer like a Kimai API,
	// typically because the base URL points to something else.
	ErrUnexpectedResponse = errors.New("kimai: unexpected response")
	// ErrInvalidURL means the configured base URL is not usable.
	ErrInvalidURL = errors.New("kimai: invalid URL")
	// ErrCertificate means the server's TLS certificate is not trusted
	// (self-signed, expired or issued for another host).
	ErrCertificate = errors.New("kimai: untrusted TLS certificate")
	// ErrNotHTTPS means an https:// URL points to a server that only speaks plain HTTP.
	ErrNotHTTPS = errors.New("kimai: server does not speak HTTPS")
)

// APIError is returned for non-2xx responses.
type APIError struct {
	StatusCode int
	// Message is the error message reported by Kimai, if any.
	Message string
	// Details are validation messages ("The end date must not be earlier …").
	Details []string
}

func (e *APIError) Error() string {
	if len(e.Details) > 0 {
		return fmt.Sprintf("kimai: HTTP %d: %s", e.StatusCode, strings.Join(e.Details, " "))
	}
	if e.Message != "" {
		return fmt.Sprintf("kimai: HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("kimai: HTTP %d", e.StatusCode)
}

// Unwrap maps well-known status codes to the sentinel errors.
func (e *APIError) Unwrap() error {
	switch e.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusNotFound:
		return ErrNotFound
	}
	return nil
}

// NetworkError wraps transport failures and unwraps to ErrOffline.
type NetworkError struct{ Err error }

func (e *NetworkError) Error() string { return "kimai: " + e.Err.Error() }

// Is reports ErrOffline so callers can use errors.Is(err, ErrOffline).
func (e *NetworkError) Is(target error) bool { return target == ErrOffline }

// Unwrap returns the underlying transport error.
func (e *NetworkError) Unwrap() error { return e.Err }
