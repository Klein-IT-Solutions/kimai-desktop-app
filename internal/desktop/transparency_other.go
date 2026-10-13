//go:build !linux || gtk3

package desktop

// MakeFramelessWindowsTransparent is a no-op where Wails already supports
// transparent windows (macOS, Windows, Linux GTK3).
func MakeFramelessWindowsTransparent() {}
