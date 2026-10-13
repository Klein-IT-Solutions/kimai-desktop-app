//go:build !windows

package desktop

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestAutostart(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPIMAGE", "/home/anna/Apps/Kimai Tracker.AppImage")
	if AutostartEnabled() {
		t.Fatal("should be disabled initially")
	}
	if err := SetAutostart(true); err != nil {
		t.Fatal(err)
	}
	if !AutostartEnabled() {
		t.Fatal("should be enabled")
	}
	path, _ := AutostartFile()
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `Exec="/home/anna/Apps/Kimai Tracker.AppImage" --hidden`) {
		t.Errorf("entry = %s", data)
	}
	if err := SetAutostart(false); err != nil || AutostartEnabled() {
		t.Fatalf("disable: %v", err)
	}
	if err := SetAutostart(false); err != nil {
		t.Errorf("disabling twice: %v", err)
	}
}

func TestDesktopExecQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/local/bin/kimai-tracker": "/usr/local/bin/kimai-tracker",
		"/opt/My App/run":              `"/opt/My App/run"`,
		`/tmp/a"b`:                     `"/tmp/a\"b"`,
	} {
		if got := desktopExecQuote(in); got != want {
			t.Errorf("quote(%q) = %q, want %q", in, got, want)
		}
	}
}
