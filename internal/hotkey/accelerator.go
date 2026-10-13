// Package hotkey registers a system-wide keyboard shortcut.
//
// On X11 the key is grabbed on the root window. On Wayland, where clients
// cannot grab keys, the XDG GlobalShortcuts portal is used (KDE Plasma,
// GNOME 48+); the desktop asks the user to confirm the shortcut. Where
// neither works, users can bind `kimai-tracker --toggle` in their desktop's
// keyboard settings instead.
package hotkey

import (
	"fmt"
	"strings"
)

// Accelerator is a parsed shortcut such as "Super+Shift+T".
type Accelerator struct {
	Super, Ctrl, Alt, Shift bool
	// Key is a lowercase letter or digit, "f1"…"f12" or "space".
	Key string
}

// Parse reads shortcuts like "Super+Shift+T", "ctrl+alt+k" or "Meta+F9".
func Parse(s string) (Accelerator, error) {
	var a Accelerator
	parts := strings.Split(strings.ReplaceAll(s, " ", ""), "+")
	for i, p := range parts {
		lp := strings.ToLower(p)
		if i < len(parts)-1 {
			switch lp {
			case "super", "meta", "win", "logo", "cmd":
				a.Super = true
			case "ctrl", "control":
				a.Ctrl = true
			case "alt", "option":
				a.Alt = true
			case "shift":
				a.Shift = true
			default:
				return Accelerator{}, fmt.Errorf("unknown modifier %q", p)
			}
			continue
		}
		switch {
		case len(lp) == 1 && (lp[0] >= 'a' && lp[0] <= 'z' || lp[0] >= '0' && lp[0] <= '9'):
			a.Key = lp
		case lp == "space":
			a.Key = lp
		case len(lp) >= 2 && lp[0] == 'f' && fNumber(lp) > 0:
			a.Key = lp
		default:
			return Accelerator{}, fmt.Errorf("unsupported key %q", p)
		}
	}
	if a.Key == "" {
		return Accelerator{}, fmt.Errorf("missing key in %q", s)
	}
	if !a.Super && !a.Ctrl && !a.Alt && fNumber(a.Key) == 0 {
		return Accelerator{}, fmt.Errorf("%q needs Super, Ctrl or Alt", s)
	}
	return a, nil
}

func fNumber(k string) int {
	var n int
	if _, err := fmt.Sscanf(k, "f%d", &n); err != nil || n < 1 || n > 12 || fmt.Sprintf("f%d", n) != k {
		return 0
	}
	return n
}

// String returns the canonical form, e.g. "Super+Shift+T".
func (a Accelerator) String() string {
	var parts []string
	if a.Super {
		parts = append(parts, "Super")
	}
	if a.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if a.Alt {
		parts = append(parts, "Alt")
	}
	if a.Shift {
		parts = append(parts, "Shift")
	}
	key := strings.ToUpper(a.Key)
	if a.Key == "space" {
		key = "Space"
	}
	return strings.Join(append(parts, key), "+")
}

// PortalTrigger returns the trigger in the XDG shortcuts format, e.g. "LOGO+SHIFT+t".
func (a Accelerator) PortalTrigger() string {
	var parts []string
	if a.Ctrl {
		parts = append(parts, "CTRL")
	}
	if a.Alt {
		parts = append(parts, "ALT")
	}
	if a.Shift {
		parts = append(parts, "SHIFT")
	}
	if a.Super {
		parts = append(parts, "LOGO")
	}
	key := a.Key
	if n := fNumber(key); n > 0 {
		key = fmt.Sprintf("F%d", n)
	}
	return strings.Join(append(parts, key), "+")
}

// keysym returns the X11 keysym of the key.
func (a Accelerator) keysym() uint32 {
	if n := fNumber(a.Key); n > 0 {
		return 0xffbe + uint32(n-1) // XK_F1…
	}
	if a.Key == "space" {
		return 0x20
	}
	return uint32(a.Key[0]) // Latin-1 keysyms equal ASCII for a-z, 0-9
}
