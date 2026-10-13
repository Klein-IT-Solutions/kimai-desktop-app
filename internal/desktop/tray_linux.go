package desktop

import "github.com/godbus/dbus/v5"

// TrayAvailable reports whether a StatusNotifierItem host is running, i.e.
// whether a tray icon will actually be visible. GNOME needs the
// "AppIndicator and KStatusNotifierItem Support" extension for this
// (pre-installed on Ubuntu).
func TrayAvailable() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	var has bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&has)
	return err == nil && has
}
