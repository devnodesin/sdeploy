//go:build !windows

package main

import (
	"os"
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

// getShellPath returns the path to the shell executable (Unix implementation)
// It first tries to find "sh" in PATH, then falls back to common shell locations
func getShellPath() string {
	// Try to find sh in PATH first
	if shellPath, err := exec.LookPath("sh"); err == nil {
		return shellPath
	}

	// Fallback to common shell locations
	commonPaths := []string{"/bin/sh", "/usr/bin/sh", "/usr/local/bin/sh"}
	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Last resort: return "sh" and let the OS handle it
	return "sh"
}

// getShellArgs returns the shell arguments for executing a command (Unix implementation)
func getShellArgs() string {
	return "-c"
}
