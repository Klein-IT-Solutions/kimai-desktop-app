package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
	"github.com/klein-it-solutions/kimai-desktop-app/internal/kimai"
)

// EventConnectionChanged is emitted with a ConnectionResult whenever the
// stored connection was checked.
const EventConnectionChanged = "connection:changed"

// Connection states reported to the frontend.
const (
	StatusOK            = "ok"
	StatusNotConfigured = "not_configured"
	StatusUnauthorized  = "unauthorized"
	StatusForbidden     = "forbidden"
	StatusOffline       = "offline"
	StatusInvalidURL    = "invalid_url"
	StatusUnexpected    = "unexpected"
	StatusKeyring       = "keyring"
	StatusError         = "error"
)

// Settings is the settings view model. The token itself is never sent to the UI.
type Settings struct {
	KimaiURL   string `json:"kimaiUrl"`
	HasToken   bool   `json:"hasToken"`
	Language   string `json:"language"`
	ConfigPath string `json:"configPath"`
	// TokenStorage is "keyring" or "file".
	TokenStorage string `json:"tokenStorage"`
	// SecretsPath is the file used when TokenStorage is "file".
	SecretsPath string `json:"secretsPath"`
	// KeyringError is set when the selected token storage cannot be read.
	KeyringError string `json:"keyringError,omitempty"`
}

// SettingsInput carries changed settings from the UI.
type SettingsInput struct {
	KimaiURL string `json:"kimaiUrl"`
	// Token replaces the stored API token; empty keeps the current one.
	Token    string `json:"token"`
	Language string `json:"language"`
	// TokenStorage switches the storage ("keyring" or "file"); empty keeps it.
	// A stored token moves along when no new token is given.
	TokenStorage string `json:"tokenStorage"`
}

// SaveResult is returned by SaveSettings.
type SaveResult struct {
	Settings Settings `json:"settings"`
	// KeyringFailed is set when the token could not be stored in the keyring.
	// The UI then offers the file fallback; the other settings were saved.
	KeyringFailed bool `json:"keyringFailed"`
	// Message explains a failure in German.
	Message string `json:"message,omitempty"`
}

// ConnectionResult describes the outcome of a connection check.
type ConnectionResult struct {
	OK            bool   `json:"ok"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	BaseURL       string `json:"baseUrl"`
	UserName      string `json:"userName,omitempty"`
	ServerVersion string `json:"serverVersion,omitempty"`
	CheckedAt     string `json:"checkedAt"`
}

// Connection holds configuration, secrets and what was learned about the
// Kimai account. It is shared by the services but not bound to the frontend.
type Connection struct {
	store   *config.Store
	keyring config.Secrets
	file    config.Secrets

	mu       sync.Mutex
	location *time.Location // Kimai user time zone, learned on connection check
}

// NewConnection creates the shared connection state. keyring is the default
// secret store, file the opt-in fallback.
func NewConnection(store *config.Store, keyring, file config.Secrets) *Connection {
	return &Connection{store: store, keyring: keyring, file: file}
}

// secrets returns the store selected in the configuration.
func (c *Connection) secrets() config.Secrets {
	return c.secretsFor(c.store.Get().TokenStorage)
}

func (c *Connection) secretsFor(storage string) config.Secrets {
	if storage == config.StorageFile {
		return c.file
	}
	return c.keyring
}

// ErrNoToken is returned when no API token is stored.
var ErrNoToken = errors.New("no API token stored")

// Client builds a Kimai client from the stored settings.
func (c *Connection) Client() (*kimai.Client, error) {
	cfg := c.store.Get()
	token, err := c.secrets().Get(config.SecretKimaiToken)
	if err != nil {
		if errors.Is(err, config.ErrSecretNotFound) {
			return nil, ErrNoToken
		}
		return nil, err
	}
	client, err := newClient(cfg.KimaiURL, token)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.location != nil {
		client.Location = c.location
	}
	c.mu.Unlock()
	return client, nil
}

func (c *Connection) setLocation(loc *time.Location) {
	c.mu.Lock()
	c.location = loc
	c.mu.Unlock()
}

func newClient(url, token string) (*kimai.Client, error) {
	return kimai.New(url, token, kimai.WithUserAgent("kimai-tracker/"+Version))
}

// SettingsService manages configuration, the API token and the connection check.
type SettingsService struct {
	conn  *Connection
	store *config.Store

	mu   sync.Mutex
	last *ConnectionResult
}

// NewSettingsService creates the service.
func NewSettingsService(conn *Connection) *SettingsService {
	return &SettingsService{conn: conn, store: conn.store}
}

// GetSettings returns the current settings.
func (s *SettingsService) GetSettings() Settings {
	cfg := s.store.Get()
	out := Settings{
		KimaiURL:     cfg.KimaiURL,
		Language:     cfg.Language,
		ConfigPath:   s.store.Path(),
		TokenStorage: storageName(cfg.TokenStorage),
	}
	if f, ok := s.conn.file.(*config.FileSecrets); ok {
		out.SecretsPath = f.Path
	}
	_, err := s.conn.secrets().Get(config.SecretKimaiToken)
	switch {
	case err == nil:
		out.HasToken = true
	case !errors.Is(err, config.ErrSecretNotFound):
		out.KeyringError = secretsErrorMessage(err)
	}
	return out
}

// SaveSettings validates and stores the settings. The API token goes to the
// selected secret store (system keyring by default), everything else to
// config.json. A keyring failure is reported in the result, not as an error,
// so the UI can offer the file fallback.
func (s *SettingsService) SaveSettings(in SettingsInput) (SaveResult, error) {
	url := strings.TrimSpace(in.KimaiURL)
	if url != "" {
		normalized, err := kimai.NormalizeBaseURL(url)
		if err != nil {
			return SaveResult{}, errors.New("Die Kimai-URL ist ungültig.")
		}
		url = normalized
	}

	cfg := s.store.Get()
	oldStorage := storageName(cfg.TokenStorage)
	newStorage := oldStorage
	if in.TokenStorage != "" {
		newStorage = storageName(in.TokenStorage)
	}

	token := strings.TrimSpace(in.Token)
	if token == "" && newStorage != oldStorage {
		// Move an existing token to the new storage.
		if old, err := s.conn.secretsFor(oldStorage).Get(config.SecretKimaiToken); err == nil {
			token = old
		}
	}
	res := SaveResult{}
	if token != "" {
		if err := s.conn.secretsFor(newStorage).Set(config.SecretKimaiToken, token); err != nil {
			res.Message = secretsErrorMessage(err)
			res.KeyringFailed = newStorage == config.StorageKeyring
			newStorage = oldStorage // keep using what worked before
		} else if newStorage != oldStorage {
			_ = s.conn.secretsFor(oldStorage).Delete(config.SecretKimaiToken)
		}
	}

	err := s.store.Update(func(c *config.Config) {
		c.KimaiURL = url
		if in.Language != "" {
			c.Language = in.Language
		}
		c.TokenStorage = newStorage
		if newStorage == config.StorageKeyring {
			c.TokenStorage = "" // default, keep config.json minimal
		}
	})
	if err != nil {
		return SaveResult{}, fmt.Errorf("Einstellungen konnten nicht gespeichert werden: %v", err)
	}
	s.mu.Lock()
	s.last = nil
	s.mu.Unlock()
	res.Settings = s.GetSettings()
	return res, nil
}

// RemoveToken deletes the stored API token.
func (s *SettingsService) RemoveToken() error {
	if err := s.conn.secrets().Delete(config.SecretKimaiToken); err != nil {
		return errors.New(secretsErrorMessage(err))
	}
	s.mu.Lock()
	s.last = nil
	s.mu.Unlock()
	return nil
}

// TestConnection checks the given URL and token without saving them. An empty
// token falls back to the stored one, so an existing token can be re-tested.
func (s *SettingsService) TestConnection(kimaiURL, token string) ConnectionResult {
	if strings.TrimSpace(token) == "" {
		stored, err := s.conn.secrets().Get(config.SecretKimaiToken)
		if err != nil && !errors.Is(err, config.ErrSecretNotFound) {
			return result(StatusKeyring, secretsErrorMessage(err), "")
		}
		token = stored
	}
	return s.check(kimaiURL, token, false)
}

// CheckConnection checks the stored settings, remembers the result and emits
// EventConnectionChanged.
func (s *SettingsService) CheckConnection() ConnectionResult {
	cfg := s.store.Get()
	token, err := s.conn.secrets().Get(config.SecretKimaiToken)
	var res ConnectionResult
	switch {
	case err != nil && !errors.Is(err, config.ErrSecretNotFound):
		res = result(StatusKeyring, secretsErrorMessage(err), cfg.KimaiURL)
	case cfg.KimaiURL == "" && token == "":
		res = result(StatusNotConfigured, "Noch keine Kimai-Verbindung eingerichtet.", cfg.KimaiURL)
	case token == "":
		res = result(StatusNotConfigured, "Kein API-Token gespeichert.", cfg.KimaiURL)
	case cfg.KimaiURL == "":
		res = result(StatusNotConfigured, "Keine Kimai-URL gespeichert.", cfg.KimaiURL)
	default:
		res = s.check(cfg.KimaiURL, token, true)
	}
	s.mu.Lock()
	s.last = &res
	s.mu.Unlock()
	if a := application.Get(); a != nil {
		a.Event.Emit(EventConnectionChanged, res)
	}
	return res
}

// LastConnection returns the most recent stored-connection check, running one
// if none happened yet.
func (s *SettingsService) LastConnection() ConnectionResult {
	s.mu.Lock()
	last := s.last
	s.mu.Unlock()
	if last != nil {
		return *last
	}
	return s.CheckConnection()
}

func (s *SettingsService) check(url, token string, remember bool) ConnectionResult {
	if strings.TrimSpace(token) == "" {
		return result(StatusUnauthorized, "Bitte einen API-Token eingeben.", url)
	}
	c, err := newClient(url, token)
	if err != nil {
		status, msg := DescribeError(err)
		return result(status, msg, url)
	}
	ctx, cancel := context.WithTimeout(context.Background(), kimai.DefaultTimeout)
	defer cancel()
	info, err := c.Check(ctx)
	if err != nil {
		status, msg := DescribeError(err)
		return result(status, msg, c.BaseURL())
	}
	if remember {
		if tz := info.User.TimeZone(); tz != "" {
			if loc, err := time.LoadLocation(tz); err == nil {
				s.conn.setLocation(loc)
			}
		}
	}
	res := result(StatusOK, "Verbunden als "+info.User.DisplayName(), c.BaseURL())
	res.OK = true
	res.UserName = info.User.DisplayName()
	res.ServerVersion = info.Version.Version
	return res
}

func result(status, message, baseURL string) ConnectionResult {
	return ConnectionResult{
		Status:    status,
		Message:   message,
		BaseURL:   baseURL,
		CheckedAt: time.Now().Format(time.RFC3339),
	}
}

const keyringMessage = "Der System-Schlüsselbund ist nicht erreichbar. Unter Linux wird ein Secret-Service-Dienst wie GNOME Keyring oder KWallet benötigt."

func storageName(s string) string {
	if s == config.StorageFile {
		return config.StorageFile
	}
	return config.StorageKeyring
}

// secretsErrorMessage explains a secret store failure, including the
// technical cause so users can report it.
func secretsErrorMessage(err error) string {
	if errors.Is(err, config.ErrKeyringUnavailable) {
		return keyringMessage + " Details: " + err.Error()
	}
	return "Token-Speicher nicht lesbar: " + err.Error()
}

// DescribeError maps client errors to a status code and a German user message.
func DescribeError(err error) (status, message string) {
	switch {
	case err == nil:
		return StatusOK, ""
	case errors.Is(err, kimai.ErrInvalidURL):
		return StatusInvalidURL, "Die Kimai-URL ist ungültig. Beispiel: https://kimai.example.com"
	case errors.Is(err, kimai.ErrUnauthorized):
		return StatusUnauthorized, "Der API-Token ist ungültig oder abgelaufen (401)."
	case errors.Is(err, kimai.ErrForbidden):
		return StatusForbidden, "Keine Berechtigung für diese Aktion (403). Prüfe die Rolle deines Kimai-Benutzers."
	case errors.Is(err, kimai.ErrNotFound):
		return StatusUnexpected, "Kimai-API nicht gefunden (404). Ist die URL korrekt?"
	case errors.Is(err, kimai.ErrNotHTTPS):
		return StatusInvalidURL, "Der Server unterstützt kein HTTPS. Verwende http:// in der URL."
	case errors.Is(err, kimai.ErrCertificate):
		return StatusOffline, "Das TLS-Zertifikat des Servers ist nicht vertrauenswürdig (selbst signiert, abgelaufen oder falscher Hostname)."
	case errors.Is(err, kimai.ErrOffline):
		return StatusOffline, "Kimai ist nicht erreichbar. Bitte Internetverbindung und URL prüfen."
	case errors.Is(err, context.DeadlineExceeded):
		return StatusOffline, "Zeitüberschreitung – Kimai antwortet nicht."
	case errors.Is(err, kimai.ErrUnexpectedResponse):
		return StatusUnexpected, "Unerwartete Antwort vom Server. Ist das die URL deiner Kimai-Installation?"
	case errors.Is(err, config.ErrKeyringUnavailable):
		return StatusKeyring, secretsErrorMessage(err)
	}
	var apiErr *kimai.APIError
	if errors.As(err, &apiErr) {
		if len(apiErr.Details) > 0 {
			return StatusError, "Kimai lehnt das ab: " + strings.Join(apiErr.Details, " ")
		}
		if apiErr.Message != "" {
			return StatusError, fmt.Sprintf("Kimai meldet einen Fehler (%d): %s", apiErr.StatusCode, apiErr.Message)
		}
		return StatusError, fmt.Sprintf("Kimai meldet einen Fehler (%d).", apiErr.StatusCode)
	}
	return StatusError, "Unbekannter Fehler: " + err.Error()
}
