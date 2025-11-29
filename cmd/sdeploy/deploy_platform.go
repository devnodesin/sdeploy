package main

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"strconv"
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

// buildCommand creates an exec.Cmd with user/group settings (Unix implementation)
// It runs the command as the specified user and group if we're root,
// or falls back to running as current user if user lookup fails
// Returns the command and a warning message (empty if no warning)
func buildCommand(ctx context.Context, command, runAsUser, runAsGroup string) (*exec.Cmd, string) {
	defaultCmd := exec.CommandContext(ctx, getShellPath(), getShellArgs(), command)

	// Check if we're running as root
	currentUser, err := user.Current()
	if err != nil {
		// Can't determine current user, run command directly
		return defaultCmd, "Unable to determine current user, running command as current user"
	}
	if currentUser.Uid != "0" {
		// Not running as root, run command directly (no warning, this is normal)
		return defaultCmd, ""
	}

	// Running as root, attempt to run as specified user/group
	targetUser, err := user.Lookup(runAsUser)
	if err != nil {
		// User not found, run command directly as current user (root)
		return defaultCmd, "User '" + runAsUser + "' not found, running command as root"
	}

	targetGroup, err := user.LookupGroup(runAsGroup)
	if err != nil {
		// Group not found, run command directly as current user (root)
		return defaultCmd, "Group '" + runAsGroup + "' not found, running command as root"
	}

	uid, err := strconv.ParseUint(targetUser.Uid, 10, 32)
	if err != nil {
		return defaultCmd, "Invalid UID for user '" + runAsUser + "', running command as root"
	}

	gid, err := strconv.ParseUint(targetGroup.Gid, 10, 32)
	if err != nil {
		return defaultCmd, "Invalid GID for group '" + runAsGroup + "', running command as root"
	}

	cmd := exec.CommandContext(ctx, getShellPath(), getShellArgs(), command)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
		Setpgid: true,
	}

	return cmd, ""
}
