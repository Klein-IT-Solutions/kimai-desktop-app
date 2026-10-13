//go:build windows

package hotkey

import "testing"

func TestWin32Codes(t *testing.T) {
	cases := map[string][2]uint32{
		"Super+Shift+T": {modWin | modShift, 'T'},
		"Ctrl+Alt+5":    {modControl | modAlt, '5'},
		"Ctrl+Space":    {modControl, 0x20},
		"F9":            {0, 0x78},
	}
	for in, want := range cases {
		a, err := Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		if mods, vk := a.win32(); mods != want[0] || vk != want[1] {
			t.Errorf("%s: got %#x/%#x, want %#x/%#x", in, mods, vk, want[0], want[1])
		}
	}
}
