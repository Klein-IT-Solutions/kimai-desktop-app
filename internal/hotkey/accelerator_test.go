package hotkey

import "testing"

func TestParse(t *testing.T) {
	cases := map[string]struct{ str, portal string }{
		"Super+Shift+T":   {"Super+Shift+T", "SHIFT+LOGO+t"},
		"ctrl + alt + k":  {"Ctrl+Alt+K", "CTRL+ALT+k"},
		"Meta+F9":         {"Super+F9", "LOGO+F9"},
		"F12":             {"F12", "F12"},
		"Shift+Ctrl+1":    {"Ctrl+Shift+1", "CTRL+SHIFT+1"},
		"Alt+Space":       {"Alt+Space", "ALT+space"},
		"Win+Shift+Space": {"Super+Shift+Space", "SHIFT+LOGO+space"},
	}
	for in, want := range cases {
		a, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q): %v", in, err)
			continue
		}
		if a.String() != want.str || a.PortalTrigger() != want.portal {
			t.Errorf("Parse(%q) = %q / %q, want %q / %q", in, a.String(), a.PortalTrigger(), want.str, want.portal)
		}
	}
	for _, bad := range []string{"", "T", "Shift+T", "Super+", "Hyper+T", "Super+Enter", "F13", "Super+F0"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
}

func TestKeysym(t *testing.T) {
	for in, want := range map[string]uint32{"Super+T": 't', "Super+1": '1', "F1": 0xffbe, "F12": 0xffc9, "Alt+Space": 0x20} {
		a, _ := Parse(in)
		if got := a.keysym(); got != want {
			t.Errorf("%s keysym = %#x, want %#x", in, got, want)
		}
	}
}
