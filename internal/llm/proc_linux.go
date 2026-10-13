package llm

import (
	"os/exec"
	"syscall"
)

// setProcAttrs makes the server die with the app, even after a crash.
func setProcAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}

func terminate(cmd *exec.Cmd) error { return cmd.Process.Signal(syscall.SIGTERM) }
