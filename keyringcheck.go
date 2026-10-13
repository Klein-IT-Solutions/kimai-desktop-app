package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/klein-it-solutions/kimai-desktop-app/internal/config"
)

// checkKeyring implements `kimai-tracker --check-keyring`: it reports which
// Secret Service provider is running and does a store/read/delete round trip
// with a throw-away value. The real API token is not touched.
func checkKeyring() int {
	fmt.Println("Kimai Tracker – keyring check")
	fmt.Printf("  Desktop:      %s (%s)\n", orDash(os.Getenv("XDG_CURRENT_DESKTOP")), orDash(os.Getenv("XDG_SESSION_TYPE")))
	fmt.Printf("  Provider:     %s\n", secretServiceProvider())

	const key = "kimai-tracker-selftest"
	const value = "selftest-value"
	var k config.Keyring
	steps := []struct {
		name string
		run  func() error
	}{
		{"store + read back", func() error { return k.Set(key, value) }},
		{"read", func() error {
			v, err := k.Get(key)
			if err == nil && v != value {
				return fmt.Errorf("got %q", v)
			}
			return err
		}},
		{"delete", func() error { return k.Delete(key) }},
	}
	ok := true
	for _, s := range steps {
		if err := s.run(); err != nil {
			fmt.Printf("  ✗ %-16s %v\n", s.name, err)
			ok = false
			break
		}
		fmt.Printf("  ✓ %s\n", s.name)
	}
	if !ok {
		fmt.Println("\nThe keyring is not usable. In the settings you can store the token in a file instead.")
		return 1
	}
	fmt.Println("\nThe keyring works.")
	return 0
}

// secretServiceProvider names the process that owns org.freedesktop.secrets.
func secretServiceProvider() string {
	conn, err := dbus.SessionBus()
	if err != nil {
		return "no D-Bus session bus: " + err.Error()
	}
	var owner string
	if err := conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, "org.freedesktop.secrets").Store(&owner); err != nil {
		return "none running (activation may start one)"
	}
	var pid uint32
	if err := conn.BusObject().Call("org.freedesktop.DBus.GetConnectionUnixProcessID", 0, owner).Store(&pid); err != nil {
		return owner
	}
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return fmt.Sprintf("pid %d", pid)
	}
	return fmt.Sprintf("%s (pid %d)", strings.TrimSpace(string(comm)), pid)
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}
