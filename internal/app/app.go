// Package app contains the services exposed to the frontend.
package app

import (
	"errors"
	"os"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/desktop"
)

// Version is overridden at build time via -ldflags "-X .../internal/app.Version=...".
var Version = "0.1.0-dev"

// Info describes the running application and its environment.
type Info struct {
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	SessionType string `json:"sessionType"` // "wayland", "x11" or "" on non-Linux
	DarkMode    bool   `json:"darkMode"`
	// WindowControls tells the frontend how to render the title bar buttons.
	WindowControls desktop.WindowControls `json:"windowControls"`
}

// AppService exposes general application information and window helpers.
type AppService struct {
	legal Legal
}

// Legal holds the license of Kimai Tracker and the third-party notices.
type Legal struct {
	License string `json:"license"`
	Notices string `json:"notices"`
}

// NewAppService creates the service with the license texts to show.
func NewAppService(license, notices string) *AppService {
	return &AppService{legal: Legal{License: license, Notices: notices}}
}

// Legal returns the license and third-party notices.
func (s *AppService) Legal() Legal { return s.legal }

// Info returns version and environment details.
func (s *AppService) Info() Info {
	return Info{
		Version:     Version,
		Platform:    runtime.GOOS,
		SessionType: SessionType(),
		DarkMode:    s.IsDarkMode(),
		// Detected per call so a changed GNOME button layout applies after a reload.
		WindowControls: desktop.DetectWindowControls(),
	}
}

// IsDarkMode reports whether the desktop prefers a dark colour scheme.
func (s *AppService) IsDarkMode() bool {
	if a := application.Get(); a != nil {
		return a.Env.IsDarkMode()
	}
	return false
}

// SessionType returns the Linux display server type ("wayland" or "x11").
func SessionType() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	if t := strings.ToLower(os.Getenv("XDG_SESSION_TYPE")); t == "wayland" || t == "x11" {
		return t
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "wayland"
	}
	if os.Getenv("DISPLAY") != "" {
		return "x11"
	}
	return ""
}

// OpenURL opens an http(s) link in the default browser.
func (s *AppService) OpenURL(url string) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return errors.New("only http(s) links can be opened")
	}
	if a := application.Get(); a != nil {
		return a.Browser.OpenURL(url)
	}
	return nil
}
