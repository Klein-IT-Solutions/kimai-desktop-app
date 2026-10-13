package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
	"github.com/klein-it-solutions/kimai-desktop-app/internal/llm"
)

// EventAssistantDownload reports model/runtime download progress (AssistantDownload).
const EventAssistantDownload = "assistant:download"

// AssistantModel is a downloadable model and whether it is installed.
type AssistantModel struct {
	llm.Model
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
}

// AssistantSettings is the view model for the local AI settings.
type AssistantSettings struct {
	Enabled       bool             `json:"enabled"`
	AutoDictation bool             `json:"autoDictation"`
	Model         string           `json:"model"`
	Server        string           `json:"server"`
	Models        []AssistantModel `json:"models"`
	// Ready is set when runtime and model are available.
	Ready bool `json:"ready"`
}

// AssistantSettingsInput carries changed settings.
type AssistantSettingsInput struct {
	Enabled       bool   `json:"enabled"`
	AutoDictation bool   `json:"autoDictation"`
	Model         string `json:"model"`
	Server        string `json:"server"`
}

// AssistantDownload reports a running download.
type AssistantDownload struct {
	Kind     string `json:"kind"` // "model" or "runtime"
	ID       string `json:"id"`
	Done     int64  `json:"done"`
	Total    int64  `json:"total"`
	Finished bool   `json:"finished"`
	Error    string `json:"error,omitempty"`
}

// AssistantService turns notes into bullet points with a local LLM.
type AssistantService struct {
	store  *config.Store
	server *llm.Server

	mu             sync.Mutex
	cancelDownload context.CancelFunc
}

// NewAssistantService creates the service.
func NewAssistantService(store *config.Store) *AssistantService {
	return &AssistantService{store: store, server: llm.NewServer()}
}

// ServiceShutdown stops the llama.cpp server.
func (a *AssistantService) ServiceShutdown() error {
	a.server.Stop()
	return nil
}

// GetSettings returns the settings, filling in a detected llama-server.
func (a *AssistantService) GetSettings() AssistantSettings {
	cfg := a.store.Get().Assistant
	server := cfg.Server
	if server == "" || !fileExists(server) {
		if found := llm.FindServer(); found != "" {
			server = found
		}
	}
	out := AssistantSettings{
		Enabled:       cfg.Enabled,
		AutoDictation: cfg.AutoDictation,
		Model:         cfg.Model,
		Server:        server,
	}
	for _, m := range llm.Models {
		st := AssistantModel{Model: m}
		if p, err := llm.ModelPath(m); err == nil {
			st.Path = p
			st.Installed = fileExists(p)
		}
		out.Models = append(out.Models, st)
	}
	out.Ready = fileExists(out.Server) && fileExists(out.Model)
	return out
}

// SaveSettings stores the settings.
func (a *AssistantService) SaveSettings(in AssistantSettingsInput) (AssistantSettings, error) {
	err := a.store.Update(func(c *config.Config) {
		c.Assistant = config.AssistantConfig{
			Enabled:       in.Enabled,
			AutoDictation: in.AutoDictation,
			Model:         strings.TrimSpace(in.Model),
			Server:        strings.TrimSpace(in.Server),
		}
	})
	if !in.Enabled {
		a.server.Stop()
	}
	return a.GetSettings(), err
}

// DownloadModel downloads a model in the background and selects it.
func (a *AssistantService) DownloadModel(id string) error {
	m, ok := llm.FindModel(id)
	if !ok {
		return errors.New("Unbekanntes Modell")
	}
	dest, err := llm.ModelPath(m)
	if err != nil {
		return err
	}
	url := m.URL
	if mirror := os.Getenv("KIMAI_TRACKER_LLM_MODEL_URL"); mirror != "" {
		url = strings.TrimRight(mirror, "/") + "/" + m.File
	}
	return a.runDownload("model", id, func(ctx context.Context, progress func(int64, int64)) error {
		if err := downloadGGUF(ctx, url, dest, progress); err != nil {
			return err
		}
		return a.store.Update(func(c *config.Config) { c.Assistant.Model = dest })
	})
}

// InstallRuntime downloads llama.cpp for this platform in the background.
func (a *AssistantService) InstallRuntime() error {
	src := llm.OfficialSource
	if mirror := os.Getenv("KIMAI_TRACKER_LLAMA_URL"); mirror != "" {
		src = llm.MirrorSource(mirror)
	}
	return a.runDownload("runtime", "llama.cpp", func(ctx context.Context, progress func(int64, int64)) error {
		server, err := llm.Install(ctx, nil, src, progress)
		if err != nil {
			return err
		}
		return a.store.Update(func(c *config.Config) { c.Assistant.Server = server })
	})
}

func (a *AssistantService) runDownload(kind, id string, run func(context.Context, func(int64, int64)) error) error {
	a.mu.Lock()
	if a.cancelDownload != nil {
		a.mu.Unlock()
		return errors.New("Es läuft bereits ein Download.")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelDownload = cancel
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.cancelDownload = nil
			a.mu.Unlock()
			cancel()
		}()
		err := run(ctx, func(done, total int64) {
			emit(EventAssistantDownload, AssistantDownload{Kind: kind, ID: id, Done: done, Total: total})
		})
		p := AssistantDownload{Kind: kind, ID: id, Finished: true}
		switch {
		case errors.Is(err, context.Canceled):
			p.Error = "Abgebrochen."
		case err != nil:
			p.Error = err.Error()
		}
		emit(EventAssistantDownload, p)
	}()
	return nil
}

// CancelDownload aborts a running download.
func (a *AssistantService) CancelDownload() {
	a.mu.Lock()
	cancel := a.cancelDownload
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Summarize turns text into bullet points. The first call loads the model,
// which can take a few seconds.
func (a *AssistantService) Summarize(text string) (string, error) {
	s := a.GetSettings()
	if !s.Enabled {
		return "", errors.New("Die lokale KI ist ausgeschaltet.")
	}
	if !s.Ready {
		return "", errors.New("Die lokale KI ist noch nicht eingerichtet – siehe Einstellungen.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	return llm.Summarize(ctx, a.server, s.Server, s.Model, text)
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func emit(name string, data any) {
	if a := application.Get(); a != nil {
		a.Event.Emit(name, data)
	}
}
