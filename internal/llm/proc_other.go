//go:build !linux

package llm

import "os/exec"

func setProcAttrs(*exec.Cmd) {}

func terminate(cmd *exec.Cmd) error { return cmd.Process.Kill() }
