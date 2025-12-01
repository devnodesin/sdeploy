package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup sets the command to run in its own process group (Unix only)
// If SysProcAttr already exists, it preserves those settings
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	} else {
		cmd.SysProcAttr.Setpgid = true
	}
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

// buildCommand creates an exec.Cmd for the given command string
// Sets umask 0022 to ensure created files are readable
func buildCommand(ctx context.Context, command string) *exec.Cmd {
	// Wrap command with umask to ensure proper file permissions for generated files
	// umask 0022 means: owner gets full permissions, group and others get read/execute
	wrappedCommand := "umask 0022 && " + command

	return exec.CommandContext(ctx, getShellPath(), getShellArgs(), wrappedCommand)
}

// ensureParentDirExists creates parent directories if they don't exist
func ensureParentDirExists(ctx context.Context, parentDir string, logger *Logger, projectName string) error {
	// Check if parent directory already exists
	if info, err := os.Stat(parentDir); err == nil {
		if info.IsDir() {
			// Directory exists, nothing to do
			return nil
		}
		return fmt.Errorf("parent path exists but is not a directory: %s", parentDir)
	}

	// Log the directory creation
	if logger != nil {
		logger.Infof(projectName, "Creating parent directory: %s", parentDir)
	}

	// Create the directory with standard permissions
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	return nil
}
