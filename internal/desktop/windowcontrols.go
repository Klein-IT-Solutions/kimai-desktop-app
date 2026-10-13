package desktop

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// Window control styles.
const (
	StyleMac     = "mac"
	StyleWindows = "windows"
	StyleGnome   = "gnome"
	StyleKDE     = "kde"
)

// Window control buttons the app supports (the window is not resizable, so
// there is no maximise button).
const (
	ButtonMinimize = "minimize"
	ButtonClose    = "close"
)

// WindowControls describes how the title bar buttons should look and where
// they go, following the platform and desktop conventions.
type WindowControls struct {
	Style string `json:"style"`
	// Side is "left" or "right".
	Side string `json:"side"`
	// Buttons in display order, a subset of "minimize" and "close".
	Buttons []string `json:"buttons"`
}

// StyleOverrideEnv forces a style for development and screenshots,
// e.g. KIMAI_TRACKER_WINDOW_STYLE=windows.
const StyleOverrideEnv = "KIMAI_TRACKER_WINDOW_STYLE"

// DetectWindowControls returns the window controls for the running platform.
// On Linux it honours the desktop's button layout (GNOME "button-layout").
func DetectWindowControls() WindowControls {
	if style := strings.ToLower(os.Getenv(StyleOverrideEnv)); style != "" {
		return defaultsFor(style)
	}
	switch runtime.GOOS {
	case "darwin":
		return defaultsFor(StyleMac)
	case "windows":
		return defaultsFor(StyleWindows)
	}
	style := linuxStyle(os.Getenv("XDG_CURRENT_DESKTOP"))
	wc := defaultsFor(style)
	if layout, ok := readButtonLayout(); ok {
		if side, buttons, ok := ParseButtonLayout(layout); ok {
			wc.Side, wc.Buttons = side, buttons
		}
	}
	return wc
}

func defaultsFor(style string) WindowControls {
	switch style {
	case StyleMac:
		return WindowControls{Style: StyleMac, Side: "left", Buttons: []string{ButtonClose, ButtonMinimize}}
	case StyleWindows:
		return WindowControls{Style: StyleWindows, Side: "right", Buttons: []string{ButtonMinimize, ButtonClose}}
	case StyleKDE:
		return WindowControls{Style: StyleKDE, Side: "right", Buttons: []string{ButtonMinimize, ButtonClose}}
	default:
		return WindowControls{Style: StyleGnome, Side: "right", Buttons: []string{ButtonMinimize, ButtonClose}}
	}
}

func linuxStyle(currentDesktop string) string {
	for _, d := range strings.Split(strings.ToUpper(currentDesktop), ":") {
		if d == "KDE" {
			return StyleKDE
		}
	}
	return StyleGnome
}

// ParseButtonLayout interprets a GNOME/GTK button layout such as
// "appmenu:minimize,maximize,close" (buttons right) or "close,minimize:"
// (buttons left). It returns the side holding the close button and the
// supported buttons on that side in order. ok is false if the layout
// contains none of the supported buttons.
func ParseButtonLayout(layout string) (side string, buttons []string, ok bool) {
	leftPart, rightPart, _ := strings.Cut(strings.ReplaceAll(layout, " ", ""), ":")
	pick := func(part string) []string {
		var out []string
		for _, b := range strings.Split(part, ",") {
			if b == ButtonMinimize || b == ButtonClose {
				out = append(out, b)
			}
		}
		return out
	}
	left, right := pick(leftPart), pick(rightPart)
	has := func(list []string, b string) bool {
		for _, x := range list {
			if x == b {
				return true
			}
		}
		return false
	}
	switch {
	case has(left, ButtonClose):
		return "left", left, true
	case has(right, ButtonClose):
		return "right", right, true
	case len(right) > 0:
		return "right", right, true
	case len(left) > 0:
		return "left", left, true
	}
	return "", nil, false
}

// readButtonLayout asks the XDG desktop portal (works inside Flatpak and on
// GNOME/KDE) and falls back to gsettings.
func readButtonLayout() (string, bool) {
	if v, ok := portalButtonLayout(); ok {
		return v, true
	}
	ctx, cancel := contextWithTimeout(time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.wm.preferences", "button-layout").Output()
	if err != nil {
		return "", false
	}
	v := strings.Trim(strings.TrimSpace(string(out)), "'")
	return v, v != ""
}

func portalButtonLayout() (string, bool) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return "", false
	}
	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	ctx, cancel := contextWithTimeout(time.Second)
	defer cancel()
	call := obj.CallWithContext(ctx, "org.freedesktop.portal.Settings.Read", 0,
		"org.gnome.desktop.wm.preferences", "button-layout")
	if call.Err != nil {
		return "", false
	}
	var v dbus.Variant
	if err := call.Store(&v); err != nil {
		return "", false
	}
	// Settings.Read wraps the value in an extra variant.
	for {
		inner, ok := v.Value().(dbus.Variant)
		if !ok {
			break
		}
		v = inner
	}
	s, ok := v.Value().(string)
	return s, ok && s != ""
}
