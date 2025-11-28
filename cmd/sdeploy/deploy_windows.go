//go:build windows
// +build windows

package main

import (
	"os/exec"
)

// setProcessGroup is a no-op on Windows as process groups work differently
func setProcessGroup(cmd *exec.Cmd) {
	// Windows doesn't support Setpgid; process groups are handled differently
}

// killProcessGroup kills the process on Windows
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}
