//go:build windows

package desktop

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestAutostartRunKey(t *testing.T) {
	// Keep a real entry of an installed app intact.
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE); err == nil {
		old, _, err := k.GetStringValue(runValue)
		k.Close()
		if err == nil {
			t.Cleanup(func() {
				if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE); err == nil {
					k.SetStringValue(runValue, old)
					k.Close()
				}
			})
		}
	}
	if err := SetAutostart(true); err != nil {
		t.Fatal(err)
	}
	if !AutostartEnabled() {
		t.Fatal("should be enabled")
	}
	k, _ := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	v, _, _ := k.GetStringValue(runValue)
	k.Close()
	if !strings.HasPrefix(v, `"`) || !strings.HasSuffix(v, `" --hidden`) {
		t.Errorf("Run value = %q", v)
	}
	if err := SetAutostart(false); err != nil || AutostartEnabled() {
		t.Fatalf("disable: %v, enabled=%v", err, AutostartEnabled())
	}
	if err := SetAutostart(false); err != nil {
		t.Fatalf("disable twice: %v", err)
	}
}
