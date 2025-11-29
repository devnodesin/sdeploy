//go:build windows

package main

import (
	"os"
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

// getShellPath returns the path to the shell executable (Windows implementation)
// On Windows, we use cmd.exe for shell command execution
func getShellPath() string {
	// Try to find cmd.exe in PATH first
	if shellPath, err := exec.LookPath("cmd.exe"); err == nil {
		return shellPath
	}

	// Fallback to common Windows shell locations
	commonPaths := []string{os.Getenv("COMSPEC"), "C:\\Windows\\System32\\cmd.exe"}

	// Add SystemRoot-based path only if SystemRoot is set
	if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
		commonPaths = append([]string{systemRoot + "\\System32\\cmd.exe"}, commonPaths...)
	}

	for _, path := range commonPaths {
		if path != "" {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}

	// Last resort: return "cmd.exe" and let the OS handle it
	return "cmd.exe"
}

// getShellArgs returns the shell arguments for executing a command (Windows implementation)
func getShellArgs() string {
	return "/c"
}
