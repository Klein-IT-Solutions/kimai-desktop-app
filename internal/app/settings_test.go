package app

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
)

func newTestService(t *testing.T) (*SettingsService, config.MemorySecrets) {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	secrets := config.MemorySecrets{}
	file := &config.FileSecrets{Path: filepath.Join(t.TempDir(), "secrets.json")}
	return NewSettingsService(NewConnection(store, secrets, file)), secrets
}

func fakeKimai(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":401,"message":"Invalid credentials"}`)
			return
		}
		switch r.URL.Path {
		case "/api/users/me":
			_, _ = io.WriteString(w, `{"id":1,"username":"anna","alias":"Anna","timezone":"Europe/Berlin"}`)
		case "/api/version":
			_, _ = io.WriteString(w, `{"version":"2.40.0"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSaveSettingsStoresTokenInSecretsOnly(t *testing.T) {
	svc, secrets := newTestService(t)
	res, err := svc.SaveSettings(SettingsInput{KimaiURL: "kimai.example.com/api/", Token: " tok "})
	if err != nil || res.KeyringFailed {
		t.Fatal(err, res.Message)
	}
	got := res.Settings
	if got.KimaiURL != "https://kimai.example.com" || !got.HasToken || got.TokenStorage != "keyring" {
		t.Errorf("settings = %+v", got)
	}
	if secrets[config.SecretKimaiToken] != "tok" {
		t.Errorf("token = %q", secrets[config.SecretKimaiToken])
	}

	// An empty token keeps the stored one.
	if _, err := svc.SaveSettings(SettingsInput{KimaiURL: "https://kimai.example.com"}); err != nil {
		t.Fatal(err)
	}
	if secrets[config.SecretKimaiToken] != "tok" {
		t.Error("token was cleared by an empty input")
	}
}

func TestSaveSettingsRejectsInvalidURL(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.SaveSettings(SettingsInput{KimaiURL: "ftp://x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestTestConnection(t *testing.T) {
	srv := fakeKimai(t)
	svc, _ := newTestService(t)

	res := svc.TestConnection(srv.URL, "good")
	if !res.OK || res.UserName != "Anna" || res.ServerVersion != "2.40.0" {
		t.Errorf("ok result = %+v", res)
	}

	res = svc.TestConnection(srv.URL, "bad")
	if res.OK || res.Status != StatusUnauthorized {
		t.Errorf("bad token result = %+v", res)
	}

	res = svc.TestConnection("not a url ::", "good")
	if res.Status != StatusInvalidURL {
		t.Errorf("invalid url result = %+v", res)
	}

	res = svc.TestConnection(srv.URL, "")
	if res.Status != StatusUnauthorized {
		t.Errorf("missing token result = %+v", res)
	}
}

func TestCheckConnectionUsesStoredSettings(t *testing.T) {
	srv := fakeKimai(t)
	svc, _ := newTestService(t)

	if res := svc.CheckConnection(); res.Status != StatusNotConfigured {
		t.Fatalf("unconfigured = %+v", res)
	}
	if _, err := svc.SaveSettings(SettingsInput{KimaiURL: srv.URL, Token: "good"}); err != nil {
		t.Fatal(err)
	}
	if res := svc.CheckConnection(); !res.OK {
		t.Fatalf("configured = %+v", res)
	}
	if res := svc.LastConnection(); !res.OK {
		t.Errorf("last = %+v", res)
	}

	c, err := svc.conn.Client()
	if err != nil {
		t.Fatal(err)
	}
	if c.Location == nil || c.Location.String() != "Europe/Berlin" {
		t.Errorf("client location = %v, want Kimai user time zone", c.Location)
	}
}

func TestOfflineMessage(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	svc, _ := newTestService(t)
	if res := svc.TestConnection(url, "good"); res.Status != StatusOffline {
		t.Errorf("result = %+v", res)
	}
}

// brokenKeyring accepts secrets but never returns them, like a Secret Service
// whose unlock prompt was dismissed.
type brokenKeyring struct{}

func (brokenKeyring) Get(string) (string, error) { return "", config.ErrSecretNotFound }
func (brokenKeyring) Set(string, string) error {
	return errors.Join(config.ErrKeyringUnavailable, errors.New("read-back failed"))
}
func (brokenKeyring) Delete(string) error { return nil }

func TestKeyringFailureOffersFileFallback(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	file := &config.FileSecrets{Path: filepath.Join(t.TempDir(), "secrets.json")}
	svc := NewSettingsService(NewConnection(store, brokenKeyring{}, file))

	res, err := svc.SaveSettings(SettingsInput{KimaiURL: "https://kimai.example.com", Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.KeyringFailed || res.Message == "" || res.Settings.HasToken {
		t.Fatalf("keyring failure not reported: %+v", res)
	}
	if store.Get().KimaiURL != "https://kimai.example.com" {
		t.Error("URL should be saved even if the token could not be stored")
	}

	res, err = svc.SaveSettings(SettingsInput{KimaiURL: "https://kimai.example.com", Token: "tok", TokenStorage: "file"})
	if err != nil || res.KeyringFailed || !res.Settings.HasToken || res.Settings.TokenStorage != "file" {
		t.Fatalf("file fallback = %+v, %v", res, err)
	}
	if v, _ := file.Get(config.SecretKimaiToken); v != "tok" {
		t.Errorf("file token = %q", v)
	}
	if _, err := svc.conn.Client(); err != nil {
		t.Errorf("client from file token: %v", err)
	}
}

func TestSwitchStorageMovesToken(t *testing.T) {
	svc, keyring := newTestService(t)
	if _, err := svc.SaveSettings(SettingsInput{KimaiURL: "https://k.example.com", Token: "tok"}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.SaveSettings(SettingsInput{KimaiURL: "https://k.example.com", TokenStorage: "file"})
	if err != nil || !res.Settings.HasToken || res.Settings.TokenStorage != "file" {
		t.Fatalf("to file = %+v, %v", res, err)
	}
	if _, ok := keyring[config.SecretKimaiToken]; ok {
		t.Error("token should be removed from the keyring after moving")
	}
	res, err = svc.SaveSettings(SettingsInput{KimaiURL: "https://k.example.com", TokenStorage: "keyring"})
	if err != nil || !res.Settings.HasToken || res.Settings.TokenStorage != "keyring" || keyring[config.SecretKimaiToken] != "tok" {
		t.Fatalf("back to keyring = %+v, %v", res, err)
	}
}

func TestCheckConnectionReportsMissingToken(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.SaveSettings(SettingsInput{KimaiURL: "https://k.example.com"}); err != nil {
		t.Fatal(err)
	}
	if res := svc.CheckConnection(); res.Status != StatusNotConfigured || res.Message != "Kein API-Token gespeichert." {
		t.Errorf("result = %+v", res)
	}
}
