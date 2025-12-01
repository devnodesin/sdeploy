package main

import (
	"context"
	"fmt"
	"os"
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
// It verifies and creates directories with standard permissions.
func runPreflightChecks(ctx context.Context, project *ProjectConfig, logger *Logger) error {
	if logger != nil {
		logger.Infof(project.Name, "Running preflight checks")
	}

	// Get effective execute_path (default to local_path if not set)
	effectiveExecutePath := getEffectiveExecutePath(project.LocalPath, project.ExecutePath)

	// Check and create local_path if needed
	if project.LocalPath != "" {
		if err := ensureDirectoryExists(project.LocalPath, logger, project.Name); err != nil {
			return fmt.Errorf("failed to ensure local_path exists: %w", err)
		}
	}

	// Check and create execute_path if needed (and different from local_path)
	if effectiveExecutePath != "" && effectiveExecutePath != project.LocalPath {
		if err := ensureDirectoryExists(effectiveExecutePath, logger, project.Name); err != nil {
			return fmt.Errorf("failed to ensure execute_path exists: %w", err)
		}
	}

	if logger != nil {
		logger.Infof(project.Name, "Preflight checks completed")
	}

	return nil
}

// ensureDirectoryExists ensures a directory exists with standard permissions (0755).
func ensureDirectoryExists(dirPath string, logger *Logger, projectName string) error {
	// Check if directory already exists
	info, err := os.Stat(dirPath)
	if err == nil {
		// Path exists
		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %s", dirPath)
		}
		// Directory exists, nothing more to do
		return nil
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

	return nil
}
