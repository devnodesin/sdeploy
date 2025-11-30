package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPreflightDirCreation tests that preflight creates missing directories
func TestPreflightDirCreation(t *testing.T) {
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "repo")
	executePath := filepath.Join(tmpDir, "www")

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")

	project := &ProjectConfig{
		Name:        "TestProject",
		LocalPath:   localPath,
		ExecutePath: executePath,
	}

	err := runPreflightChecks(context.Background(), project, logger)
	if err != nil {
		t.Fatalf("Preflight checks failed: %v", err)
	}

	// Verify directories were created
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		t.Error("Expected local_path to be created")
	}
	if _, err := os.Stat(executePath); os.IsNotExist(err) {
		t.Error("Expected execute_path to be created")
	}

	// Verify logging
	logOutput := buf.String()
	if !strings.Contains(logOutput, "Creating directory:") {
		t.Errorf("Expected log message about creating directory, got: %s", logOutput)
	}
}

// TestPreflightExistingDirs tests that preflight handles existing directories
func TestPreflightExistingDirs(t *testing.T) {
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "repo")
	executePath := filepath.Join(tmpDir, "www")

	// Create directories beforehand
	if err := os.MkdirAll(localPath, 0755); err != nil {
		t.Fatalf("Failed to create localPath: %v", err)
	}
	if err := os.MkdirAll(executePath, 0755); err != nil {
		t.Fatalf("Failed to create executePath: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")

	project := &ProjectConfig{
		Name:        "TestProject",
		LocalPath:   localPath,
		ExecutePath: executePath,
	}

	err := runPreflightChecks(context.Background(), project, logger)
	if err != nil {
		t.Fatalf("Preflight checks failed: %v", err)
	}

	// Should NOT log creating directories since they already exist
	logOutput := buf.String()
	if strings.Contains(logOutput, "Creating directory:") {
		t.Errorf("Should not create directories that already exist, got: %s", logOutput)
	}
}

// TestPreflightExecutePathDefault tests that execute_path defaults to local_path
func TestPreflightExecutePathDefault(t *testing.T) {
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "repo")

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")

	project := &ProjectConfig{
		Name:        "TestProject",
		LocalPath:   localPath,
		ExecutePath: "", // Not set - should default to local_path
	}

	err := runPreflightChecks(context.Background(), project, logger)
	if err != nil {
		t.Fatalf("Preflight checks failed: %v", err)
	}

	// Verify local_path was created (execute_path defaults to it)
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		t.Error("Expected local_path to be created")
	}
}

// TestPreflightNestedDirs tests creation of nested directories
func TestPreflightNestedDirs(t *testing.T) {
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "deep", "nested", "path", "repo")
	executePath := filepath.Join(tmpDir, "another", "deep", "path", "www")

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")

	project := &ProjectConfig{
		Name:        "TestProject",
		LocalPath:   localPath,
		ExecutePath: executePath,
	}

	err := runPreflightChecks(context.Background(), project, logger)
	if err != nil {
		t.Fatalf("Preflight checks failed: %v", err)
	}

	// Verify nested directories were created
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		t.Error("Expected nested local_path to be created")
	}
	if _, err := os.Stat(executePath); os.IsNotExist(err) {
		t.Error("Expected nested execute_path to be created")
	}
}

// TestPreflightEmptyPaths tests handling of empty paths
func TestPreflightEmptyPaths(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "")

	project := &ProjectConfig{
		Name:        "TestProject",
		LocalPath:   "",
		ExecutePath: "",
	}

	// Should not fail for empty paths - just skip checking
	err := runPreflightChecks(context.Background(), project, logger)
	if err != nil {
		t.Fatalf("Preflight checks should not fail for empty paths: %v", err)
	}
}

// TestPreflightFileConflict tests error when path exists as file
func TestPreflightFileConflict(t *testing.T) {
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "conflict")

	// Create a file where directory should be
	if err := os.WriteFile(localPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")

	project := &ProjectConfig{
		Name:        "TestProject",
		LocalPath:   localPath,
		ExecutePath: tmpDir,
	}

	err := runPreflightChecks(context.Background(), project, logger)
	if err == nil {
		t.Error("Expected error when path is a file, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("Expected 'not a directory' error, got: %v", err)
	}
}

// TestPreflightLogging tests comprehensive logging
func TestPreflightLogging(t *testing.T) {
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "repo")

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")

	project := &ProjectConfig{
		Name:        "TestProject",
		LocalPath:   localPath,
		ExecutePath: localPath, // Same as local_path
	}

	err := runPreflightChecks(context.Background(), project, logger)
	if err != nil {
		t.Fatalf("Preflight checks failed: %v", err)
	}

	logOutput := buf.String()

	// Should see preflight check message
	if !strings.Contains(logOutput, "Running preflight checks") {
		t.Errorf("Expected 'Running preflight checks' in log, got: %s", logOutput)
	}
}

// TestPreflightPermissions tests directory permissions (0755)
func TestPreflightPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "repo")

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")

	project := &ProjectConfig{
		Name:        "TestProject",
		LocalPath:   localPath,
		ExecutePath: localPath,
	}

	err := runPreflightChecks(context.Background(), project, logger)
	if err != nil {
		t.Fatalf("Preflight checks failed: %v", err)
	}

	// Check directory permissions
	info, err := os.Stat(localPath)
	if err != nil {
		t.Fatalf("Failed to stat local_path: %v", err)
	}

	perm := info.Mode().Perm()
	// Should be at least readable and executable by owner, group, and others (755)
	if perm&0755 != 0755 {
		t.Errorf("Expected directory permissions 0755, got %o", perm)
	}
}

// TestPreflightWithRunAsUser tests preflight with custom run_as_user
func TestPreflightWithRunAsUser(t *testing.T) {
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "repo")

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")

	project := &ProjectConfig{
		Name:        "TestProject",
		LocalPath:   localPath,
		ExecutePath: localPath,
		RunAsUser:   "testuser",
		RunAsGroup:  "testgroup",
	}

	err := runPreflightChecks(context.Background(), project, logger)
	if err != nil {
		t.Fatalf("Preflight checks failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		t.Error("Expected local_path to be created")
	}
}

// TestGetEffectiveExecutePath tests the helper function for execute_path defaulting
func TestGetEffectiveExecutePath(t *testing.T) {
	tests := []struct {
		name        string
		localPath   string
		executePath string
		expected    string
	}{
		{
			name:        "execute_path set",
			localPath:   "/var/repo",
			executePath: "/var/www",
			expected:    "/var/www",
		},
		{
			name:        "execute_path empty, defaults to local_path",
			localPath:   "/var/repo",
			executePath: "",
			expected:    "/var/repo",
		},
		{
			name:        "both empty",
			localPath:   "",
			executePath: "",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getEffectiveExecutePath(tt.localPath, tt.executePath)
			if result != tt.expected {
				t.Errorf("getEffectiveExecutePath(%q, %q) = %q, want %q",
					tt.localPath, tt.executePath, result, tt.expected)
			}
		})
	}
}

// TestPreflightCompletionLogging tests that preflight completion is logged
func TestPreflightCompletionLogging(t *testing.T) {
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "repo")

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")

	project := &ProjectConfig{
		Name:        "TestProject",
		LocalPath:   localPath,
		ExecutePath: localPath,
	}

	err := runPreflightChecks(context.Background(), project, logger)
	if err != nil {
		t.Fatalf("Preflight checks failed: %v", err)
	}

	logOutput := buf.String()

	// Should see preflight completion message
	if !strings.Contains(logOutput, "Preflight checks completed") {
		t.Errorf("Expected 'Preflight checks completed' in log, got: %s", logOutput)
	}
}
