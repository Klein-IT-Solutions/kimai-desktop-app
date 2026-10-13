// Package config loads and stores the user configuration in
// ~/.config/kimai-tracker/config.json. Secrets (API tokens, keys) are never
// written to this file; see secrets.go.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// AppDirName is the directory name below the user config dir.
const AppDirName = "kimai-tracker"

// Config holds all non-secret settings.
type Config struct {
	// KimaiURL is the base URL of the Kimai instance, e.g. https://kimai.example.com.
	KimaiURL string `json:"kimaiUrl"`
	// Language is the UI / transcription language as ISO 639-1 code.
	Language string `json:"language"`
	// TokenStorage is where secrets live: "" / "keyring" (default) or "file".
	TokenStorage string `json:"tokenStorage,omitempty"`
	// Hotkey is the global shortcut to show/hide the window ("" disables it).
	Hotkey string `json:"hotkey"`
	// Notifications enables desktop notifications on start/stop.
	Notifications bool `json:"notifications"`
	// Voice configures speech input.
	Voice VoiceConfig `json:"voice"`
	// Assistant configures the local language model for bullet points.
	Assistant AssistantConfig `json:"assistant"`
}

// AssistantConfig configures the local LLM (llama.cpp).
type AssistantConfig struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model,omitempty"`  // path to a GGUF file
	Server  string `json:"server,omitempty"` // path to llama-server
	// AutoDictation turns dictated text into bullet points automatically.
	AutoDictation bool `json:"autoDictation"`
}

// Transcription engines.
const (
	EngineOff   = ""
	EngineLocal = "local" // whisper.cpp
	EngineAPI   = "api"   // OpenAI-compatible API
)

// VoiceConfig configures recording and transcription.
type VoiceConfig struct {
	Engine        string `json:"engine"`
	WhisperBinary string `json:"whisperBinary,omitempty"`
	WhisperModel  string `json:"whisperModel,omitempty"`
	APIURL        string `json:"apiUrl,omitempty"`
	APIModel      string `json:"apiModel,omitempty"`
	// PushToTalk is the global hold-to-talk shortcut ("" disables it).
	PushToTalk string `json:"pushToTalk"`
}

// DefaultPushToTalk is held down while speaking. Windows reserves most
// Win+Shift combinations (Win+Shift+V cycles notifications), so it gets
// Ctrl+Alt shortcuts.
var DefaultPushToTalk = platformDefault("Super+Shift+V", "Ctrl+Alt+V")

// DefaultHotkey toggles the window.
var DefaultHotkey = platformDefault("Super+Shift+T", "Ctrl+Alt+T")

func platformDefault(other, windows string) string {
	if runtime.GOOS == "windows" {
		return windows
	}
	return other
}

// Token storage backends.
const (
	StorageKeyring = "keyring"
	StorageFile    = "file"
)

// Default returns the configuration used when no file exists yet.
func Default() Config {
	return Config{
		Language:      "de",
		Hotkey:        DefaultHotkey,
		Notifications: true,
		Voice:         VoiceConfig{APIURL: "https://api.openai.com/v1", APIModel: "whisper-1", PushToTalk: DefaultPushToTalk},
	}
}

// Store reads and writes the configuration file. It is safe for concurrent use.
type Store struct {
	path string
	mu   sync.Mutex
	cfg  Config
}

// DefaultPath returns ~/.config/kimai-tracker/config.json (honouring XDG_CONFIG_HOME).
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine config directory: %w", err)
	}
	return filepath.Join(dir, AppDirName, "config.json"), nil
}

// Open loads the configuration at path. A missing file yields the defaults.
func Open(path string) (*Store, error) {
	s := &Store{path: path, cfg: Default()}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &s.cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if s.cfg.Language == "" {
		s.cfg.Language = Default().Language
	}
	return s, nil
}

// Path returns the location of the configuration file.
func (s *Store) Path() string { return s.path }

// Get returns a copy of the current configuration.
func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Update applies fn to the configuration and persists the result atomically.
func (s *Store) Update(fn func(*Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg
	fn(&next)
	if err := s.write(next); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

func (s *Store) write(cfg Config) error {
	if err := SaveJSON(s.path, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
