//go:build !linux

package desktop

// TrayAvailable is always true on macOS and Windows.
func TrayAvailable() bool { return true }
