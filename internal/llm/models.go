// Package llm runs a small local language model through llama.cpp's
// llama-server to turn notes into bullet points. Model and runtime are
// downloaded on demand; nothing has to be installed by hand.
package llm

import (
	"os"
	"path/filepath"
	"runtime"
)

// Model is a downloadable GGUF chat model.
type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	File        string `json:"file"`
	SizeMB      int    `json:"sizeMb"`
	License     string `json:"license"`
}

// Models are small instruction-tuned models that run on laptop CPUs.
var Models = []Model{
	{
		ID: "qwen2.5-0.5b", Name: "Qwen2.5 0.5B", Description: "Sehr klein und schnell, einfache Stichpunkte",
		URL:  "https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q4_k_m.gguf",
		File: "qwen2.5-0.5b-instruct-q4_k_m.gguf", SizeMB: 490, License: "Apache-2.0",
	},
	{
		ID: "qwen2.5-1.5b", Name: "Qwen2.5 1.5B", Description: "Empfohlen – gutes Deutsch, wenige Sekunden",
		URL:  "https://huggingface.co/Qwen/Qwen2.5-1.5B-Instruct-GGUF/resolve/main/qwen2.5-1.5b-instruct-q4_k_m.gguf",
		File: "qwen2.5-1.5b-instruct-q4_k_m.gguf", SizeMB: 1120, License: "Apache-2.0",
	},
	{
		ID: "llama3.2-3b", Name: "Llama 3.2 3B", Description: "Formuliert am besten, braucht ca. 3 GB RAM",
		URL:  "https://huggingface.co/bartowski/Llama-3.2-3B-Instruct-GGUF/resolve/main/Llama-3.2-3B-Instruct-Q4_K_M.gguf",
		File: "Llama-3.2-3B-Instruct-Q4_K_M.gguf", SizeMB: 2020, License: "Llama 3.2 Community License",
	},
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
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
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
	return filepath.Join(dir, "llm", m.File), nil
}
