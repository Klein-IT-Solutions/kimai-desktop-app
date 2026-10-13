package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
	"github.com/klein-it-solutions/kimai-desktop-app/internal/hotkey"
	"github.com/klein-it-solutions/kimai-desktop-app/internal/voice"
)

// Voice events.
const (
	EventVoiceState    = "voice:state"    // VoiceState
	EventVoiceLevel    = "voice:level"    // float64, 0…1
	EventVoiceResult   = "voice:result"   // string, recognised text
	EventVoiceDownload = "voice:download" // DownloadProgress
)

// ModelStatus is a downloadable model and whether it is installed.
type ModelStatus struct {
	voice.Model
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
}

// DownloadProgress reports a running model download.
type DownloadProgress struct {
	ID       string `json:"id"`
	Done     int64  `json:"done"`
	Total    int64  `json:"total"`
	Finished bool   `json:"finished"`
	Error    string `json:"error,omitempty"`
}

// Voice states.
const (
	VoiceIdle         = "idle"
	VoiceRecording    = "recording"
	VoiceTranscribing = "transcribing"
)

// minRecording discards accidental taps.
const minRecording = 400 * time.Millisecond

// VoiceState is the current speech-input state.
type VoiceState struct {
	State string `json:"state"`
	// Error is set when the last recording or transcription failed.
	Error string `json:"error,omitempty"`
}

// VoiceSettings is the speech settings view model (the API key is never sent).
type VoiceSettings struct {
	Engine        string        `json:"engine"`
	WhisperBinary string        `json:"whisperBinary"`
	WhisperModel  string        `json:"whisperModel"`
	APIURL        string        `json:"apiUrl"`
	APIModel      string        `json:"apiModel"`
	HasAPIKey     bool          `json:"hasApiKey"`
	PushToTalk    hotkey.Status `json:"pushToTalk"`
	Language      string        `json:"language"`
}

// VoiceSettingsInput carries changed speech settings.
type VoiceSettingsInput struct {
	Engine        string `json:"engine"`
	WhisperBinary string `json:"whisperBinary"`
	WhisperModel  string `json:"whisperModel"`
	APIURL        string `json:"apiUrl"`
	APIModel      string `json:"apiModel"`
	// APIKey replaces the stored key; empty keeps it.
	APIKey     string `json:"apiKey"`
	PushToTalk string `json:"pushToTalk"`
}

// VoiceService records speech and transcribes it into the description.
type VoiceService struct {
	store *config.Store
	conn  *Connection
	shell *Shell

	recorder *voice.Recorder
	ptt      *hotkey.Manager

	mu             sync.Mutex
	state          VoiceState
	started        time.Time
	cancel         context.CancelFunc
	cancelDownload context.CancelFunc
}

// NewVoiceService creates the service.
func NewVoiceService(store *config.Store, conn *Connection, shell *Shell) *VoiceService {
	v := &VoiceService{store: store, conn: conn, shell: shell, state: VoiceState{State: VoiceIdle}}
	v.recorder = &voice.Recorder{
		OnLevel: func(l float64) { v.emit(EventVoiceLevel, l) },
		OnLimit: func() { go v.Stop() },
	}
	v.ptt = hotkey.NewManager(
		hotkey.Shortcut{ID: "push-to-talk", Description: "Kimai Tracker – Spracheingabe (gedrückt halten)"},
		func() { _ = v.Start() },
		func() { go v.Stop() },
		func(hotkey.Status) {},
	)
	return v
}

// ServiceStartup registers the push-to-talk shortcut.
func (v *VoiceService) ServiceStartup(context.Context, application.ServiceOptions) error {
	go v.registerPushToTalk()
	return nil
}

// ServiceShutdown releases the shortcut and the microphone.
func (v *VoiceService) ServiceShutdown() error {
	v.ptt.Stop()
	v.Cancel()
	voice.ReleaseEmbedded()
	return nil
}

func (v *VoiceService) registerPushToTalk() hotkey.Status {
	cfg := v.store.Get().Voice
	if cfg.Engine == config.EngineOff {
		v.ptt.Stop()
		return hotkey.Status{Shortcut: cfg.PushToTalk, Message: "Spracheingabe ist ausgeschaltet."}
	}
	return v.ptt.Register(cfg.PushToTalk)
}

// GetSettings returns the speech settings.
func (v *VoiceService) GetSettings() VoiceSettings {
	cfg := v.store.Get()
	_, err := v.conn.secrets().Get(config.SecretTranscriptionKey)
	return VoiceSettings{
		Engine:        cfg.Voice.Engine,
		WhisperBinary: cfg.Voice.WhisperBinary,
		WhisperModel:  cfg.Voice.WhisperModel,
		APIURL:        cfg.Voice.APIURL,
		APIModel:      cfg.Voice.APIModel,
		HasAPIKey:     err == nil,
		PushToTalk:    v.ptt.Status(),
		Language:      cfg.Language,
	}
}

// SaveSettings stores the speech settings and re-registers push-to-talk.
func (v *VoiceService) SaveSettings(in VoiceSettingsInput) (VoiceSettings, error) {
	switch in.Engine {
	case config.EngineOff, config.EngineLocal, config.EngineAPI:
	default:
		return v.GetSettings(), fmt.Errorf("Unbekannte Transkription: %s", in.Engine)
	}
	ptt := strings.TrimSpace(in.PushToTalk)
	if ptt != "" {
		acc, err := hotkey.Parse(ptt)
		if err != nil {
			return v.GetSettings(), fmt.Errorf("Ungültiges Tastenkürzel: %v", err)
		}
		ptt = acc.String()
	}
	if key := strings.TrimSpace(in.APIKey); key != "" {
		if err := v.conn.secrets().Set(config.SecretTranscriptionKey, key); err != nil {
			return v.GetSettings(), errors.New(secretsErrorMessage(err))
		}
	}
	err := v.store.Update(func(c *config.Config) {
		c.Voice = config.VoiceConfig{
			Engine:        in.Engine,
			WhisperBinary: strings.TrimSpace(in.WhisperBinary),
			WhisperModel:  strings.TrimSpace(in.WhisperModel),
			APIURL:        strings.TrimSpace(in.APIURL),
			APIModel:      strings.TrimSpace(in.APIModel),
			PushToTalk:    ptt,
		}
	})
	if err != nil {
		return v.GetSettings(), err
	}
	v.registerPushToTalk()
	return v.GetSettings(), nil
}

// Models lists the downloadable whisper.cpp models.
func (v *VoiceService) Models() []ModelStatus {
	out := make([]ModelStatus, 0, len(voice.Models))
	for _, m := range voice.Models {
		st := ModelStatus{Model: m}
		if p, err := voice.ModelPath(m); err == nil {
			st.Path = p
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				st.Installed = true
			}
		}
		out = append(out, st)
	}
	return out
}

// DownloadModel downloads a model in the background (progress via
// EventVoiceDownload) and selects it once finished.
func (v *VoiceService) DownloadModel(id string) error {
	m, ok := voice.FindModel(id)
	if !ok {
		return fmt.Errorf("Unbekanntes Modell: %s", id)
	}
	dest, err := voice.ModelPath(m)
	if err != nil {
		return err
	}
	v.mu.Lock()
	if v.cancelDownload != nil {
		v.mu.Unlock()
		return errors.New("Es läuft bereits ein Download.")
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.cancelDownload = cancel
	v.mu.Unlock()

	go func() {
		defer func() {
			v.mu.Lock()
			v.cancelDownload = nil
			v.mu.Unlock()
			cancel()
		}()
		base := voice.ModelBaseURL
		if u := os.Getenv("KIMAI_TRACKER_MODEL_URL"); u != "" { // mirror, e.g. in company networks
			base = strings.TrimRight(u, "/")
		}
		err := voice.Download(ctx, nil, base+"/"+m.File, dest, func(done, total int64) {
			v.emit(EventVoiceDownload, DownloadProgress{ID: id, Done: done, Total: total})
		})
		p := DownloadProgress{ID: id, Finished: true}
		switch {
		case errors.Is(err, context.Canceled):
			p.Error = "Abgebrochen."
		case err != nil:
			p.Error = err.Error()
		default:
			_ = v.store.Update(func(c *config.Config) { c.Voice.WhisperModel = dest })
		}
		v.emit(EventVoiceDownload, p)
	}()
	return nil
}

// CancelDownload aborts a running model download.
func (v *VoiceService) CancelDownload() {
	v.mu.Lock()
	cancel := v.cancelDownload
	v.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// DetectBinary looks for whisper.cpp's CLI and returns its path ("" if not found).
func (v *VoiceService) DetectBinary() string { return voice.FindBinary() }

// RemoveAPIKey deletes the stored transcription API key.
func (v *VoiceService) RemoveAPIKey() error {
	return v.conn.secrets().Delete(config.SecretTranscriptionKey)
}

// CheckSettings validates the configured engine without recording.
func (v *VoiceService) CheckSettings() (string, error) {
	cfg := v.store.Get().Voice
	switch cfg.Engine {
	case config.EngineLocal:
		if cfg.WhisperBinary != "" {
			if err := (voice.Local{Binary: cfg.WhisperBinary, Model: cfg.WhisperModel}).Check(); err != nil {
				return "", err
			}
			return "Externes whisper-cli und Modell gefunden.", nil
		}
		if err := (voice.Embedded{Model: cfg.WhisperModel}).Check(); err != nil {
			return "", err
		}
		return "Bereit – whisper.cpp ist in die App eingebaut.", nil
	case config.EngineAPI:
		if err := (voice.API{BaseURL: cfg.APIURL}).Check(); err != nil {
			return "", err
		}
		if _, err := v.conn.secrets().Get(config.SecretTranscriptionKey); err != nil {
			return "Konfiguration in Ordnung (ohne API-Key – nur für Server ohne Anmeldung).", nil
		}
		return "Konfiguration in Ordnung.", nil
	}
	return "", errors.New("Spracheingabe ist ausgeschaltet.")
}

// BrowseFile opens a file dialog and returns the chosen path ("" if cancelled).
func (v *VoiceService) BrowseFile(title string) (string, error) {
	a := application.Get()
	if a == nil {
		return "", nil
	}
	return a.Dialog.OpenFile().SetTitle(title).CanChooseFiles(true).PromptForSingleSelection()
}

// State returns the current speech-input state.
func (v *VoiceService) State() VoiceState {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.state
}

// Start begins recording.
func (v *VoiceService) Start() error {
	if v.store.Get().Voice.Engine == config.EngineOff {
		err := errors.New("Spracheingabe ist nicht eingerichtet – siehe Einstellungen.")
		v.setState(VoiceState{State: VoiceIdle, Error: err.Error()})
		return err
	}
	v.mu.Lock()
	busy := v.state.State != VoiceIdle
	v.mu.Unlock()
	if busy {
		return nil
	}
	if err := v.recorder.Start(); err != nil {
		v.setState(VoiceState{State: VoiceIdle, Error: err.Error()})
		return err
	}
	v.mu.Lock()
	v.started = time.Now()
	v.mu.Unlock()
	v.setState(VoiceState{State: VoiceRecording})
	return nil
}

// Stop ends the recording, transcribes it and emits the text.
func (v *VoiceService) Stop() (string, error) {
	v.mu.Lock()
	if v.state.State != VoiceRecording {
		v.mu.Unlock()
		return "", nil
	}
	started := v.started
	v.mu.Unlock()

	samples, err := v.recorder.Stop()
	if err != nil {
		v.setState(VoiceState{State: VoiceIdle})
		return "", nil
	}
	if time.Since(started) < minRecording || len(samples) < voice.SampleRate/4 {
		v.setState(VoiceState{State: VoiceIdle, Error: "Zu kurz – zum Sprechen gedrückt halten oder zweimal klicken."})
		return "", nil
	}
	v.setState(VoiceState{State: VoiceTranscribing})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	v.mu.Lock()
	v.cancel = cancel
	v.mu.Unlock()
	defer cancel()

	text, err := v.transcribe(ctx, voice.EncodeWAV(samples, voice.SampleRate))
	if err != nil {
		msg := err.Error()
		switch {
		case errors.Is(err, voice.ErrNoSpeech):
			msg = "Keine Sprache erkannt."
		case errors.Is(err, context.Canceled):
			msg = ""
		case errors.Is(err, context.DeadlineExceeded):
			msg = "Die Transkription hat zu lange gedauert."
		}
		v.setState(VoiceState{State: VoiceIdle, Error: msg})
		return "", err
	}
	v.setState(VoiceState{State: VoiceIdle})
	v.emit(EventVoiceResult, text)
	// Dictated while the window is hidden (push-to-talk): confirm it.
	if v.shell != nil && v.shell.window != nil && !v.shell.window.IsVisible() {
		v.shell.notifyAlways("Diktiert", text)
	}
	return text, nil
}

// Cancel aborts a recording or transcription without inserting text.
func (v *VoiceService) Cancel() {
	if v.recorder.Recording() {
		_, _ = v.recorder.Stop()
	}
	v.mu.Lock()
	cancel := v.cancel
	v.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	v.setState(VoiceState{State: VoiceIdle})
}

func (v *VoiceService) transcribe(ctx context.Context, wav []byte) (string, error) {
	cfg := v.store.Get()
	var t voice.Transcriber
	switch cfg.Voice.Engine {
	case config.EngineLocal:
		if cfg.Voice.WhisperBinary != "" {
			t = voice.Local{Binary: cfg.Voice.WhisperBinary, Model: cfg.Voice.WhisperModel}
		} else {
			t = voice.Embedded{Model: cfg.Voice.WhisperModel}
		}
	case config.EngineAPI:
		key, _ := v.conn.secrets().Get(config.SecretTranscriptionKey)
		t = voice.API{BaseURL: cfg.Voice.APIURL, APIKey: key, Model: cfg.Voice.APIModel}
	default:
		return "", errors.New("Spracheingabe ist nicht eingerichtet.")
	}
	return t.Transcribe(ctx, wav, cfg.Language)
}

func (v *VoiceService) setState(s VoiceState) {
	v.mu.Lock()
	v.state = s
	if s.State == VoiceIdle {
		v.cancel = nil
	}
	v.mu.Unlock()
	v.emit(EventVoiceState, s)
}

func (v *VoiceService) emit(name string, data any) {
	if a := application.Get(); a != nil {
		a.Event.Emit(name, data)
	}
}
