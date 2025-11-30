package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// setProcessGroup sets the command to run in its own process group (Unix only)
// If SysProcAttr already exists (e.g., with credentials), it preserves those settings
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

// buildCommand creates an exec.Cmd with user/group settings (Unix implementation)
// It runs the command as the specified user and group if we're root,
// or falls back to running as current user if user lookup fails
// Sets umask 0022 to ensure created files are readable by the web server
// Returns the command and a warning message (empty if no warning)
func buildCommand(ctx context.Context, command, runAsUser, runAsGroup string) (*exec.Cmd, string) {
	// Wrap command with umask to ensure proper file permissions for generated files
	// umask 0022 means: owner gets full permissions, group and others get read/execute
	wrappedCommand := "umask 0022 && " + command

	defaultCmd := exec.CommandContext(ctx, getShellPath(), getShellArgs(), wrappedCommand)

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

	cmd := exec.CommandContext(ctx, getShellPath(), getShellArgs(), wrappedCommand)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
		Setpgid: true,
	}

	return cmd, ""
}

// ensureParentDirExists creates parent directories if they don't exist and ensures
// they are owned by the specified user/group. This is needed for git clone to work
// when running as a different user.
func ensureParentDirExists(ctx context.Context, parentDir, runAsUser, runAsGroup string, logger *Logger, projectName string) error {
	// Check if parent directory already exists
	if info, err := os.Stat(parentDir); err == nil {
		if info.IsDir() {
			// Directory exists, nothing to do
			return nil
		}
		return fmt.Errorf("parent path exists but is not a directory: %s", parentDir)
	}

	// Check if we're running as root
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("unable to determine current user: %v", err)
	}

	// Log the directory creation
	if logger != nil {
		logger.Infof(projectName, "Creating parent directory: %s", parentDir)
	}

	// Create the directory
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// If we're running as root, chown the directory to the target user/group
	if currentUser.Uid == "0" {
		targetUser, err := user.Lookup(runAsUser)
		if err != nil {
			if logger != nil {
				logger.Warnf(projectName, "User '%s' not found, directory owned by root", runAsUser)
			}
			return nil
		}

		targetGroup, err := user.LookupGroup(runAsGroup)
		if err != nil {
			if logger != nil {
				logger.Warnf(projectName, "Group '%s' not found, directory owned by root", runAsGroup)
			}
			return nil
		}

		uid, err := strconv.Atoi(targetUser.Uid)
		if err != nil {
			return fmt.Errorf("invalid UID for user '%s': %v", runAsUser, err)
		}

		gid, err := strconv.Atoi(targetGroup.Gid)
		if err != nil {
			return fmt.Errorf("invalid GID for group '%s': %v", runAsGroup, err)
		}

		if logger != nil {
			logger.Infof(projectName, "Setting ownership of %s to %s:%s", parentDir, runAsUser, runAsGroup)
		}

		if err := os.Chown(parentDir, uid, gid); err != nil {
			return fmt.Errorf("failed to chown directory: %v", err)
		}
	}

	return nil
}
