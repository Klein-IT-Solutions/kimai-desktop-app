//go:build !windows

package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// AutostartFile returns ~/.config/autostart/kimai-tracker.desktop.
func AutostartFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "kimai-tracker.desktop"), nil
}

// AutostartEnabled reports whether the autostart entry exists and is not disabled.
func AutostartEnabled() bool {
	path, err := AutostartFile()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "Hidden=true" || strings.TrimSpace(line) == "X-GNOME-Autostart-enabled=false" {
			return false
		}
	}
	return true
}

// SetAutostart creates or removes the XDG autostart entry. The app starts
// hidden in the tray (--hidden).
func SetAutostart(enabled bool) error {
	if runtime.GOOS != "linux" {
		return ErrUnsupported
	}
	path, err := AutostartFile()
	if err != nil {
		return err
	}
	if !enabled {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	exe, err := executablePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(autostartEntry(exe)), 0o644)
}

func autostartEntry(exe string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Kimai Tracker
Comment=Minimalist desktop time tracker for Kimai
Exec=%s --hidden
Icon=kimai-tracker
Terminal=false
Categories=Office;Utility;
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=3
`, desktopExecQuote(exe))
}

// executablePath prefers the AppImage path, so autostart survives the
// AppImage's temporary mount point changing between runs.
func executablePath() (string, error) {
	if p := os.Getenv("APPIMAGE"); p != "" {
		return p, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// desktopExecQuote quotes a path for the Exec key of a .desktop file.
func desktopExecQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"'\\$`") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	return `"` + r.Replace(s) + `"`
}
