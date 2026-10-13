package fetch

import (
	"path/filepath"
	"testing"
)

func TestSafeJoinClamps(t *testing.T) {
	for in, want := range map[string]string{
		"../../etc/passwd":    "/data/etc/passwd",
		"llama-b1/bin/server": "/data/llama-b1/bin/server",
		`..\..\win.ini`:       "/data/win.ini",
	} {
		want = filepath.FromSlash(want)
		if p, err := safeJoin(filepath.FromSlash("/data"), in); err != nil || p != want {
			t.Errorf("safeJoin(%q) = %q, %v; want %q", in, p, err, want)
		}
	}
}
