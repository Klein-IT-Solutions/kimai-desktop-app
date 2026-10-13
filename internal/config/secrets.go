package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/zalando/go-keyring"
)

// KeyringService is the service name under which secrets are stored.
const KeyringService = "kimai-tracker"

// Secret identifiers.
const (
	SecretKimaiToken = "kimai-api-token"
	// SecretTranscriptionKey is the key for the OpenAI-compatible transcription API.
	SecretTranscriptionKey = "transcription-api-key"
)

// ErrSecretNotFound is returned when no secret is stored under a key.
var ErrSecretNotFound = errors.New("secret not found")

// ErrKeyringUnavailable is returned when the system keyring cannot be reached
// (e.g. no Secret Service provider such as GNOME Keyring or KWallet is running).
var ErrKeyringUnavailable = errors.New("system keyring unavailable")

// Secrets stores sensitive values outside the config file.
type Secrets interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

// Keyring stores secrets in the operating system keyring
// (Secret Service on Linux, Keychain on macOS, Credential Manager on Windows).
type Keyring struct{}

// Get returns the secret or ErrSecretNotFound.
func (Keyring) Get(key string) (string, error) {
	v, err := keyring.Get(KeyringService, key)
	return v, wrapKeyringErr(err)
}

// Set stores the secret and reads it back. Some Secret Service providers
// (or a dismissed unlock prompt) report success without storing anything;
// the read-back turns that into an error instead of a silently lost token.
func (Keyring) Set(key, value string) error {
	if err := wrapKeyringErr(keyring.Set(KeyringService, key, value)); err != nil {
		return err
	}
	got, err := keyring.Get(KeyringService, key)
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		return fmt.Errorf("%w: the keyring accepted the secret but does not return it (unlock prompt dismissed or collection locked?)", ErrKeyringUnavailable)
	case err != nil:
		return fmt.Errorf("%w: read-back failed: %v", ErrKeyringUnavailable, err)
	case got != value:
		return fmt.Errorf("%w: read-back returned a different value", ErrKeyringUnavailable)
	}
	return nil
}

// Delete removes the secret. Deleting a missing secret is not an error.
func (Keyring) Delete(key string) error {
	err := wrapKeyringErr(keyring.Delete(KeyringService, key))
	if errors.Is(err, ErrSecretNotFound) {
		return nil
	}
	return err
}

func wrapKeyringErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, keyring.ErrNotFound):
		return ErrSecretNotFound
	default:
		return fmt.Errorf("%w: %v", ErrKeyringUnavailable, err)
	}
}

// MemorySecrets is an in-memory Secrets implementation for tests.
type MemorySecrets map[string]string

// Get implements Secrets.
func (m MemorySecrets) Get(key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", ErrSecretNotFound
	}
	return v, nil
}

// Set implements Secrets.
func (m MemorySecrets) Set(key, value string) error { m[key] = value; return nil }

// Delete implements Secrets.
func (m MemorySecrets) Delete(key string) error { delete(m, key); return nil }

// FileSecrets stores secrets unencrypted in a JSON file readable only by the
// user (mode 0600). It is an explicit opt-in fallback for systems without a
// working keyring.
type FileSecrets struct {
	Path string
	mu   sync.Mutex
}

// DefaultSecretsPath returns ~/.config/kimai-tracker/secrets.json.
func DefaultSecretsPath() (string, error) {
	p, err := DefaultPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "secrets.json"), nil
}

func (f *FileSecrets) load() (map[string]string, error) {
	data, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", f.Path, err)
	}
	return m, nil
}

func (f *FileSecrets) save(m map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.Path)
}

// Get implements Secrets.
func (f *FileSecrets) Get(key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return "", err
	}
	v, ok := m[key]
	if !ok {
		return "", ErrSecretNotFound
	}
	return v, nil
}

// Set implements Secrets.
func (f *FileSecrets) Set(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return err
	}
	m[key] = value
	return f.save(m)
}

// Delete implements Secrets.
func (f *FileSecrets) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := m[key]; !ok {
		return nil
	}
	delete(m, key)
	if len(m) == 0 {
		return os.Remove(f.Path)
	}
	return f.save(m)
}
