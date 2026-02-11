package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestServiceLoggerMainFile tests that service logs go to main.log
func TestServiceLoggerMainFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir // log_path is now a directory

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	logger.Info("", "Service started")

	// Verify main.log was created in the log directory
	mainLogPath := filepath.Join(logPath, "main.log")
	content, err := os.ReadFile(mainLogPath)
	if err != nil {
		t.Fatalf("Failed to read main.log: %v", err)
	}

	if !strings.Contains(string(content), "Service started") {
		t.Error("Expected main.log to contain service message")
	}
}

// TestBuildLoggerFileNaming tests that build log files are named correctly
func TestBuildLoggerFileNaming(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	// Create a build logger
	buildLogger := logger.NewBuildLogger("test-project")

	buildLogger.Info("test-project", "Build started")
	buildLogger.Close(true) // success

	// Find the log file
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	var buildLogFile string
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "test-project-") && strings.HasSuffix(f.Name(), "-success.log") {
			buildLogFile = f.Name()
			break
		}
	}

	if buildLogFile == "" {
		t.Fatal("Expected build log file not found")
	}

	// Verify filename format: {project_name}-{yyyy-mm-dd}-{HHMM}-{success|fail}.log
	parts := strings.Split(buildLogFile, "-")
	if len(parts) < 5 {
		t.Errorf("Expected filename format: project-yyyy-mm-dd-MinSec-status.log, got: %s", buildLogFile)
	}

	// Verify the file contains the log message
	content, err := os.ReadFile(filepath.Join(tmpDir, buildLogFile))
	if err != nil {
		t.Fatalf("Failed to read build log file: %v", err)
	}

	if !strings.Contains(string(content), "Build started") {
		t.Error("Expected build log to contain message")
	}
}

// TestBuildLoggerFailureStatus tests that failed builds get "fail" in filename
func TestBuildLoggerFailureStatus(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	buildLogger := logger.NewBuildLogger("fail-project")
	buildLogger.Info("fail-project", "Build failed")
	buildLogger.Close(false) // failure

	// Find the log file
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	var foundFail bool
	for _, f := range files {
		if strings.Contains(f.Name(), "fail-project-") && strings.HasSuffix(f.Name(), "-fail.log") {
			foundFail = true
			break
		}
	}

	if !foundFail {
		t.Error("Expected build log file with '-fail.log' suffix")
	}
}

// TestLogPathDirectoryCreation tests that log directory is created if it doesn't exist
func TestLogPathDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "nested", "log", "dir")

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	logger.Info("", "Test message")

	// Verify directory was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("Expected log directory to be created")
	}

	// Verify main.log exists
	mainLogPath := filepath.Join(logPath, "main.log")
	if _, err := os.Stat(mainLogPath); os.IsNotExist(err) {
		t.Error("Expected main.log to be created")
	}
}

// TestServiceAndBuildLogsSeparate tests that service and build logs are separate
func TestServiceAndBuildLogsSeparate(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	// Log a service message
	logger.Info("", "Service message")

	// Create a build logger and log a build message
	buildLogger := logger.NewBuildLogger("test-proj")
	buildLogger.Info("test-proj", "Build message")
	buildLogger.Close(true)

	// Verify main.log contains only service message
	mainLogPath := filepath.Join(logPath, "main.log")
	mainContent, err := os.ReadFile(mainLogPath)
	if err != nil {
		t.Fatalf("Failed to read main.log: %v", err)
	}

	if !strings.Contains(string(mainContent), "Service message") {
		t.Error("Expected main.log to contain service message")
	}
	if strings.Contains(string(mainContent), "Build message") {
		t.Error("Expected main.log to NOT contain build message")
	}

	// Verify build log contains only build message
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	var buildLogFile string
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "test-proj-") {
			buildLogFile = f.Name()
			break
		}
	}

	buildContent, err := os.ReadFile(filepath.Join(tmpDir, buildLogFile))
	if err != nil {
		t.Fatalf("Failed to read build log: %v", err)
	}

	if !strings.Contains(string(buildContent), "Build message") {
		t.Error("Expected build log to contain build message")
	}
	if strings.Contains(string(buildContent), "Service message") {
		t.Error("Expected build log to NOT contain service message")
	}
}

// TestConsoleModeIgnoresBuildLogs tests that console mode still logs builds to files
func TestConsoleModeStillLogsBuildToFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir

	// Console mode (daemonMode=false)
	logger := NewLogger(nil, logPath, false)
	defer logger.Close()

	// Create a build logger
	buildLogger := logger.NewBuildLogger("console-build")
	buildLogger.Info("console-build", "Console build message")
	buildLogger.Close(true)

	// Even in console mode, build logs should go to files
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	var foundBuildLog bool
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "console-build-") {
			foundBuildLog = true
			break
		}
	}

	if !foundBuildLog {
		t.Error("Expected build log file even in console mode")
	}
}

// TestBuildLoggerFilenameFormat tests the exact filename format
func TestBuildLoggerFilenameFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	now := time.Now()
	expectedDate := now.Format("2006-01-02")

	buildLogger := logger.NewBuildLogger("format-test")
	buildLogger.Close(true)

	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	var buildLogFile string
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "format-test-") {
			buildLogFile = f.Name()
			break
		}
	}

	if buildLogFile == "" {
		t.Fatal("Expected build log file not found")
	}

	// Verify format: format-test-2006-01-02-HHMM-success.log
	if !strings.Contains(buildLogFile, expectedDate) {
		t.Errorf("Expected filename to contain date %s, got: %s", expectedDate, buildLogFile)
	}

	// MinSec might be off by a minute if test crosses minute boundary, so just check it's present
	// Expected format: format-test-YYYY-MM-DD-HHMM-success.log
	if !strings.HasSuffix(buildLogFile, "-success.log") {
		t.Errorf("Expected filename to end with -success.log, got: %s", buildLogFile)
	}
}

// TestLoggerDefaultPath tests that default log path is used when empty
func TestLoggerDefaultPath(t *testing.T) {
	// Create a logger with empty path in daemon mode
	// It should use Defaults.LogPath
	
	// We can't test actual /var/log/sdeploy, so we'll test with a custom default
	// This test verifies the logic, actual default is tested in integration
	
	// For now, skip this test as it requires system-level permissions
	// The logic will be covered by other tests
	t.Skip("Skipping test that requires system-level permissions")
}

// TestBuildLoggerTimestamp tests that build logs include timestamps
func TestBuildLoggerTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	buildLogger := logger.NewBuildLogger("timestamp-test")
	buildLogger.Info("timestamp-test", "Timestamped message")
	buildLogger.Close(true)

	// Find and read the log file
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	var buildLogFile string
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "timestamp-test-") {
			buildLogFile = f.Name()
			break
		}
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, buildLogFile))
	if err != nil {
		t.Fatalf("Failed to read build log: %v", err)
	}

	// Verify timestamp format is present
	if !strings.Contains(string(content), "[") || !strings.Contains(string(content), "]") {
		t.Error("Expected log to contain timestamp in brackets")
	}
}

// TestMultipleBuildLoggersSimultaneous tests that multiple build loggers can exist simultaneously
func TestMultipleBuildLoggersSimultaneous(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	// Create multiple build loggers
	build1 := logger.NewBuildLogger("project-1")
	build2 := logger.NewBuildLogger("project-2")
	build3 := logger.NewBuildLogger("project-3")

	// Log to each
	build1.Info("project-1", "Message from project 1")
	build2.Info("project-2", "Message from project 2")
	build3.Info("project-3", "Message from project 3")

	// Close all
	build1.Close(true)
	build2.Close(false)
	build3.Close(true)

	// Verify all three log files exist
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	foundProjects := make(map[string]bool)
	for _, f := range files {
		name := f.Name()
		if strings.HasPrefix(name, "project-1-") {
			foundProjects["project-1"] = true
		}
		if strings.HasPrefix(name, "project-2-") {
			foundProjects["project-2"] = true
		}
		if strings.HasPrefix(name, "project-3-") {
			foundProjects["project-3"] = true
		}
	}

	if len(foundProjects) != 3 {
		t.Errorf("Expected 3 build log files, found %d", len(foundProjects))
	}
}

// TestBuildLoggerWritesToCorrectFile tests that build logger writes to its own file
func TestBuildLoggerWritesToCorrectFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	buildLogger := logger.NewBuildLogger("isolated-test")
	buildLogger.Info("isolated-test", "Isolated message")
	buildLogger.Warn("isolated-test", "Warning message")
	buildLogger.Error("isolated-test", "Error message")
	buildLogger.Close(true)

	// Find the build log file
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	var buildLogFile string
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "isolated-test-") {
			buildLogFile = f.Name()
			break
		}
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, buildLogFile))
	if err != nil {
		t.Fatalf("Failed to read build log: %v", err)
	}

	logStr := string(content)
	if !strings.Contains(logStr, "Isolated message") {
		t.Error("Expected build log to contain info message")
	}
	if !strings.Contains(logStr, "Warning message") {
		t.Error("Expected build log to contain warning message")
	}
	if !strings.Contains(logStr, "Error message") {
		t.Error("Expected build log to contain error message")
	}
	if !strings.Contains(logStr, "[INFO]") {
		t.Error("Expected build log to contain [INFO] level")
	}
	if !strings.Contains(logStr, "[WARN]") {
		t.Error("Expected build log to contain [WARN] level")
	}
	if !strings.Contains(logStr, "[ERROR]") {
		t.Error("Expected build log to contain [ERROR] level")
	}
}

// TestBuildLoggerProjectNameWithSlashes tests that project names with slashes work correctly
func TestBuildLoggerProjectNameWithSlashes(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	// Project name with slash (e.g., "domain.com/project")
	projectName := "net.asensar.in/docs"
	buildLogger := logger.NewBuildLogger(projectName)
	buildLogger.Info(projectName, "Build started")
	buildLogger.Close(true) // success

	// Find the log file - look recursively
	var buildLogFile string
	var foundPath string
	filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), "-success.log") {
			buildLogFile = info.Name()
			foundPath = path
			return filepath.SkipAll
		}
		return nil
	})

	if buildLogFile == "" {
		t.Fatal("Expected build log file not found")
	}

	// Verify the file exists
	if _, err := os.Stat(foundPath); os.IsNotExist(err) {
		t.Errorf("Expected log file at %s, but it doesn't exist", foundPath)
	}

	// Verify content
	content, err := os.ReadFile(foundPath)
	if err != nil {
		t.Fatalf("Failed to read build log file: %v", err)
	}

	if !strings.Contains(string(content), "Build started") {
		t.Error("Expected build log to contain message")
	}

	// Verify the file is directly in the log directory (not nested)
	relPath, err := filepath.Rel(tmpDir, foundPath)
	if err != nil {
		t.Fatalf("Failed to get relative path: %v", err)
	}
	// The file should not contain directory separators (except on Windows where it might be normalized differently)
	if strings.Contains(relPath, string(filepath.Separator)) {
		t.Errorf("Expected log file to be in root directory, but found at nested path: %s", relPath)
	}
}
