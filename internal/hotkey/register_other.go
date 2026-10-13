//go:build !linux && !windows

package hotkey

import "context"

func register(_ context.Context, acc Accelerator, _ Shortcut, _, _ func(), _ func(Status)) Status {
	// macOS support follows with the platform port.
	return Status{Shortcut: acc.String(), Message: "Globale Tastenkürzel werden auf diesem System noch nicht unterstützt."}
}
