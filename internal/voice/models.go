package voice

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/fetch"
)

// ModelBaseURL hosts the official whisper.cpp ggml models.
const ModelBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"

// Model is a downloadable whisper.cpp model.
type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	File        string `json:"file"`
	SizeMB      int    `json:"sizeMb"`
}

// Models are quantised multilingual models that run well on laptops.
var Models = []Model{
	{ID: "base-q5_1", Name: "Base", Description: "Schnell, gut für kurze Notizen", File: "ggml-base-q5_1.bin", SizeMB: 57},
	{ID: "small-q5_1", Name: "Small", Description: "Empfohlen – deutlich genauer", File: "ggml-small-q5_1.bin", SizeMB: 181},
	{ID: "medium-q5_0", Name: "Medium", Description: "Sehr genau, braucht mehr Zeit und RAM", File: "ggml-medium-q5_0.bin", SizeMB: 514},
}

// FindModel returns the model with the given ID.
func FindModel(id string) (Model, bool) {
	for _, m := range Models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// DataDir returns ~/.local/share/kimai-tracker (honouring XDG_DATA_HOME).
func DataDir() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "kimai-tracker"), nil
	}
	switch runtime.GOOS {
	case "darwin", "windows":
		d, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "kimai-tracker"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "kimai-tracker"), nil
}

// ModelPath returns where a downloaded model is stored.
func ModelPath(m Model) (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "models", m.File), nil
}

// Download fetches a whisper.cpp model (see fetch.File).
func Download(ctx context.Context, client *http.Client, url, dest string, progress func(done, total int64)) error {
	err := fetch.File(ctx, client, url, dest, fetch.MagicGGML, progress)
	if errors.Is(err, fetch.ErrWrongType) {
		return errors.New("Die heruntergeladene Datei ist kein whisper.cpp-Modell")
	}
	return err
}

// binaryNames are the names whisper.cpp's CLI is installed under
// (upstream, Homebrew, distribution packages).
var binaryNames = []string{"whisper-cli", "whisper-cpp", "whisper.cpp", "whisper-cpp-cli"}

// FindBinary looks for whisper.cpp's command-line tool in PATH, next to the
// app and in the app's data directory. It returns "" if none is found.
func FindBinary() string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if d, err := DataDir(); err == nil {
		dirs = append(dirs, filepath.Join(d, "bin"))
	}
	dirs = append(dirs, "/usr/local/bin", "/opt/homebrew/bin", "/home/linuxbrew/.linuxbrew/bin")
	for _, name := range binaryNames {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		for _, d := range dirs {
			p := filepath.Join(d, name)
			if info, err := os.Stat(p); err == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0) {
				return p
			}
		}
	}
	return ""
}
