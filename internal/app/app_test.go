package app

import (
	"runtime"
	"testing"
)

func TestSessionType(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("session type is only detected on Linux")
	}
	cases := []struct {
		name, xdg, wayland, display, want string
	}{
		{"xdg wayland", "wayland", "", "", "wayland"},
		{"xdg x11 uppercase", "X11", "", ":0", "x11"},
		{"wayland display fallback", "tty", "wayland-0", ":0", "wayland"},
		{"x11 display fallback", "", "", ":0", "x11"},
		{"headless", "", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_SESSION_TYPE", c.xdg)
			t.Setenv("WAYLAND_DISPLAY", c.wayland)
			t.Setenv("DISPLAY", c.display)
			if got := SessionType(); got != c.want {
				t.Fatalf("SessionType() = %q, want %q", got, c.want)
			}
		})
	}
}
