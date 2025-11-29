//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// setProcessGroup sets the command to run in its own process group (Unix only)
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the process group (Unix only)
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
