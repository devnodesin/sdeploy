package main

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"strconv"
)

// getEffectiveExecutePath returns the effective execute_path for a project.
// If execute_path is empty, it defaults to local_path.
func getEffectiveExecutePath(localPath, executePath string) string {
	if executePath != "" {
		return executePath
	}
	return localPath
}

// runPreflightChecks performs pre-flight directory checks before deployment.
// It verifies and creates directories with correct ownership and permissions.
func runPreflightChecks(ctx context.Context, project *ProjectConfig, logger *Logger) error {
	if logger != nil {
		logger.Infof(project.Name, "Running preflight checks")
	}

	// Get effective user/group for ownership
	runAsUser, runAsGroup := getEffectiveRunAs(project)

	// Get effective execute_path (default to local_path if not set)
	effectiveExecutePath := getEffectiveExecutePath(project.LocalPath, project.ExecutePath)

	// Check and create local_path if needed
	if project.LocalPath != "" {
		if err := ensureDirectoryExists(project.LocalPath, runAsUser, runAsGroup, logger, project.Name); err != nil {
			return fmt.Errorf("failed to ensure local_path exists: %w", err)
		}
	}

	// Check and create execute_path if needed (and different from local_path)
	if effectiveExecutePath != "" && effectiveExecutePath != project.LocalPath {
		if err := ensureDirectoryExists(effectiveExecutePath, runAsUser, runAsGroup, logger, project.Name); err != nil {
			return fmt.Errorf("failed to ensure execute_path exists: %w", err)
		}
	}

	if logger != nil {
		logger.Infof(project.Name, "Preflight checks completed")
	}

	return nil
}

// ensureDirectoryExists ensures a directory exists with correct ownership and permissions.
// If running as root, it will create the directory and set ownership to the specified user/group.
// If not running as root, it will create the directory with current user ownership.
func ensureDirectoryExists(dirPath, runAsUser, runAsGroup string, logger *Logger, projectName string) error {
	// Check if directory already exists
	info, err := os.Stat(dirPath)
	if err == nil {
		// Path exists
		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %s", dirPath)
		}
		// Directory exists, optionally check/fix ownership if running as root
		return ensureDirectoryOwnership(dirPath, runAsUser, runAsGroup, logger, projectName)
	}

	if !os.IsNotExist(err) {
		// Some other error occurred
		return fmt.Errorf("failed to stat directory: %w", err)
	}

	// Directory does not exist, create it
	if logger != nil {
		logger.Infof(projectName, "Creating directory: %s", dirPath)
	}

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Set ownership if running as root
	return setDirectoryOwnership(dirPath, runAsUser, runAsGroup, logger, projectName)
}

// ensureDirectoryOwnership checks and optionally fixes directory ownership when running as root.
func ensureDirectoryOwnership(dirPath, runAsUser, runAsGroup string, logger *Logger, projectName string) error {
	// Check if we're running as root
	currentUser, err := user.Current()
	if err != nil {
		// Can't determine current user, skip ownership check
		return nil
	}

	if currentUser.Uid != "0" {
		// Not running as root, skip ownership check
		return nil
	}

	// Running as root, check and fix ownership if needed
	targetUser, err := user.Lookup(runAsUser)
	if err != nil {
		if logger != nil {
			logger.Warnf(projectName, "User '%s' not found, skipping ownership check for %s", runAsUser, dirPath)
		}
		return nil
	}

	targetGroup, err := user.LookupGroup(runAsGroup)
	if err != nil {
		if logger != nil {
			logger.Warnf(projectName, "Group '%s' not found, skipping ownership check for %s", runAsGroup, dirPath)
		}
		return nil
	}

	targetUID, err := strconv.Atoi(targetUser.Uid)
	if err != nil {
		return fmt.Errorf("invalid UID for user '%s': %w", runAsUser, err)
	}

	targetGID, err := strconv.Atoi(targetGroup.Gid)
	if err != nil {
		return fmt.Errorf("invalid GID for group '%s': %w", runAsGroup, err)
	}

	// Get current ownership
	info, err := os.Stat(dirPath)
	if err != nil {
		return fmt.Errorf("failed to stat directory: %w", err)
	}

	// Get current UID/GID from file info
	stat := info.Sys()
	if stat == nil {
		return nil // Can't get system info, skip ownership check
	}

	currentUID, currentGID := getFileOwnership(stat)

	// Check if ownership needs to be fixed
	if currentUID != targetUID || currentGID != targetGID {
		if logger != nil {
			logger.Infof(projectName, "Fixing ownership of %s to %s:%s", dirPath, runAsUser, runAsGroup)
		}
		if err := os.Chown(dirPath, targetUID, targetGID); err != nil {
			return fmt.Errorf("failed to chown directory: %w", err)
		}
	}

	return nil
}

// setDirectoryOwnership sets the ownership of a newly created directory.
func setDirectoryOwnership(dirPath, runAsUser, runAsGroup string, logger *Logger, projectName string) error {
	// Check if we're running as root
	currentUser, err := user.Current()
	if err != nil {
		if logger != nil {
			logger.Warnf(projectName, "Unable to determine current user, directory owned by current user")
		}
		return nil
	}

	if currentUser.Uid != "0" {
		// Not running as root, directory will be owned by current user
		return nil
	}

	// Running as root, set ownership
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
		return fmt.Errorf("invalid UID for user '%s': %w", runAsUser, err)
	}

	gid, err := strconv.Atoi(targetGroup.Gid)
	if err != nil {
		return fmt.Errorf("invalid GID for group '%s': %w", runAsGroup, err)
	}

	if logger != nil {
		logger.Infof(projectName, "Setting ownership of %s to %s:%s", dirPath, runAsUser, runAsGroup)
	}

	if err := os.Chown(dirPath, uid, gid); err != nil {
		return fmt.Errorf("failed to chown directory: %w", err)
	}

	return nil
}
