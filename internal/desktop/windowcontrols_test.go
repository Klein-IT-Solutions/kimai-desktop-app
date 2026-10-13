package desktop

import (
	"reflect"
	"testing"
)

func TestParseButtonLayout(t *testing.T) {
	cases := []struct {
		layout  string
		side    string
		buttons []string
		ok      bool
	}{
		{"appmenu:close", "right", []string{"close"}, true}, // GNOME default
		{"appmenu:minimize,maximize,close", "right", []string{"minimize", "close"}, true},
		{"close,minimize,maximize:", "left", []string{"close", "minimize"}, true},
		{"close,maximize,minimize:appmenu", "left", []string{"close", "minimize"}, true},
		{"icon:minimize,maximize,close", "right", []string{"minimize", "close"}, true},
		{" menu : minimize , close ", "right", []string{"minimize", "close"}, true},
		{"minimize:close", "right", []string{"close"}, true},
		{"appmenu:", "", nil, false},
		{"", "", nil, false},
	}
	for _, c := range cases {
		side, buttons, ok := ParseButtonLayout(c.layout)
		if side != c.side || !reflect.DeepEqual(buttons, c.buttons) || ok != c.ok {
			t.Errorf("ParseButtonLayout(%q) = %q, %v, %v; want %q, %v, %v", c.layout, side, buttons, ok, c.side, c.buttons, c.ok)
		}
	}
}

func TestLinuxStyle(t *testing.T) {
	for desktop, want := range map[string]string{
		"KDE":           StyleKDE,
		"ubuntu:GNOME":  StyleGnome,
		"GNOME":         StyleGnome,
		"XFCE":          StyleGnome,
		"":              StyleGnome,
		"Unity:KDE:foo": StyleKDE,
	} {
		if got := linuxStyle(desktop); got != want {
			t.Errorf("linuxStyle(%q) = %q, want %q", desktop, got, want)
		}
	}
}

func TestStyleOverride(t *testing.T) {
	t.Setenv(StyleOverrideEnv, "Windows")
	if wc := DetectWindowControls(); wc.Style != StyleWindows || wc.Side != "right" {
		t.Errorf("override = %+v", wc)
	}
	t.Setenv(StyleOverrideEnv, "mac")
	if wc := DetectWindowControls(); wc.Style != StyleMac || wc.Side != "left" || wc.Buttons[0] != ButtonClose {
		t.Errorf("override = %+v", wc)
	}
}
