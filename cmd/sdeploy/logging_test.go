package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestLoggerStdout tests logging to stdout
func TestLoggerStdout(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)

	logger.Info("TestProject", "This is an info message")

	output := buf.String()
	if !strings.Contains(output, "[INFO]") {
		t.Error("Expected log output to contain [INFO]")
	}
	if !strings.Contains(output, "[TestProject]") {
		t.Error("Expected log output to contain [TestProject]")
	}
	if !strings.Contains(output, "This is an info message") {
		t.Error("Expected log output to contain the message")
	}
}

// TestLoggerLevels tests all log levels
func TestLoggerLevels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)

	logger.Info("Project", "info message")
	logger.Warn("Project", "warn message")
	logger.Error("Project", "error message")

	output := buf.String()
	if !strings.Contains(output, "[INFO]") {
		t.Error("Expected log output to contain [INFO]")
	}
	if !strings.Contains(output, "[WARN]") {
		t.Error("Expected log output to contain [WARN]")
	}
	if !strings.Contains(output, "[ERROR]") {
		t.Error("Expected log output to contain [ERROR]")
	}
}

// TestLoggerFileOutput tests logging to a file
func TestLoggerFileOutput(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	logger.Info("TestProject", "File log message")

	// Read the file and verify content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "File log message") {
		t.Error("Expected log file to contain the message")
	}
	if !strings.Contains(string(content), "[INFO]") {
		t.Error("Expected log file to contain [INFO]")
	}
}

// TestLoggerFormat tests the log format: [TIMESTAMP] [LEVEL] [PROJECT] message
func TestLoggerFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)

	logger.Info("MyProject", "test message")

	output := buf.String()

	// Format should be: [TIMESTAMP] [LEVEL] [PROJECT] message
	// Verify structure - timestamp should be in brackets at the start
	if !strings.HasPrefix(output, "[") {
		t.Error("Log output should start with timestamp in brackets")
	}

	// Check for presence of all components
	parts := strings.Split(output, "]")
	if len(parts) < 4 {
		t.Errorf("Expected at least 4 bracket-delimited parts, got %d", len(parts))
	}
}

// TestLoggerThreadSafety tests thread safety of logging
func TestLoggerThreadSafety(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)

	var wg sync.WaitGroup
	numGoroutines := 10
	numMessages := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				logger.Info("Project", "message from goroutine")
			}
		}(i)
	}

	wg.Wait()

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	expectedLines := numGoroutines * numMessages
	if len(lines) != expectedLines {
		t.Errorf("Expected %d log lines, got %d", expectedLines, len(lines))
	}
}

// TestLoggerAppendToFile tests that logger appends to existing file
func TestLoggerAppendToFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "append.log")

	// Create initial content
	err := os.WriteFile(logPath, []byte("Initial content\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create initial log file: %v", err)
	}

	// Create logger and write
	logger := NewLogger(nil, logPath, true)
	logger.Info("Project", "Appended message")
	logger.Close()

	// Read and verify
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "Initial content") {
		t.Error("Expected log file to contain initial content")
	}
	if !strings.Contains(string(content), "Appended message") {
		t.Error("Expected log file to contain appended message")
	}
}

// TestLoggerWithTimestamp tests that logs include timestamps
func TestLoggerWithTimestamp(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)

	logger.Info("Project", "test")

	output := buf.String()
	// Check for date pattern (YYYY-MM-DD or similar)
	if !strings.Contains(output, "-") || !strings.Contains(output, ":") {
		t.Error("Expected log output to contain timestamp with date and time separators")
	}
}

// TestLoggerEmptyProject tests logging with empty project name
func TestLoggerEmptyProject(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)

	logger.Info("", "message without project")

	output := buf.String()
	// Empty project should omit the project brackets entirely
	if strings.Contains(output, "[]") {
		t.Error("Expected log output to NOT contain empty brackets '[]'")
	}
	if !strings.Contains(output, "message without project") {
		t.Error("Expected log output to contain the message")
	}
	// Should have format: [TIMESTAMP] [LEVEL] message
	if !strings.Contains(output, "[INFO] message without project") {
		t.Errorf("Expected format '[INFO] message', got: %s", output)
	}
}

// TestLoggerCreatesParentDir tests that logger creates parent directories if they don't exist
func TestLoggerCreatesParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a path with nested directories that don't exist yet
	logPath := filepath.Join(tmpDir, "nested", "subdir", "daemon.log")

	logger := NewLogger(nil, logPath, true)
	defer logger.Close()

	logger.Info("TestProject", "Message to nested log file")

	// Verify the file was created
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "Message to nested log file") {
		t.Error("Expected log file to contain the message")
	}

	// Verify directories were created
	nestedDir := filepath.Join(tmpDir, "nested")
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("Expected nested directory to be created")
	}

	subdirDir := filepath.Join(tmpDir, "nested", "subdir")
	if _, err := os.Stat(subdirDir); os.IsNotExist(err) {
		t.Error("Expected subdir directory to be created")
	}
}

// TestLoggerFallbackOnPermissionError tests that logger falls back to stderr on permission error
func TestLoggerFallbackOnPermissionError(t *testing.T) {
	// Use a path that should fail (root-owned directory without write permission)
	// On most systems, /root or similar won't be writable by normal users
	invalidPath := "/nonexistent_root_dir_12345/test.log"

	// Capture stderr to verify error message
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logger := NewLogger(nil, invalidPath, true)
	defer logger.Close()

	// Write a test message
	logger.Info("Test", "fallback message")

	// Restore stderr and read captured output
	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	buf.ReadFrom(r)
	stderrOutput := buf.String()

	// Verify error was reported to stderr
	if !strings.Contains(stderrOutput, "[SDeploy] Log file error") {
		t.Errorf("Expected error message in stderr, got: %s", stderrOutput)
	}

	// Verify logger continues to work (writes to stderr)
	if !strings.Contains(stderrOutput, "fallback message") {
		t.Errorf("Expected fallback message in stderr output, got: %s", stderrOutput)
	}
}

// TestLoggerErrorReportingContent tests the content of error messages
func TestLoggerErrorReportingContent(t *testing.T) {
	invalidPath := "/nonexistent_dir_xyz/subdir/test.log"

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logger := NewLogger(nil, invalidPath, true)
	defer logger.Close()

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	buf.ReadFrom(r)
	stderrOutput := buf.String()

	// Check for key information in error message
	requiredParts := []string{
		"Log file error",
		"Path:",
		"Error:",
		"Attempted permissions:",
		"Suggestions:",
		"Fallback: Logging to console",
	}

	for _, part := range requiredParts {
		if !strings.Contains(stderrOutput, part) {
			t.Errorf("Expected error message to contain '%s', got: %s", part, stderrOutput)
		}
	}
}

// TestLoggerNoPanicOnError tests that logger does not panic on file errors
func TestLoggerNoPanicOnError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Logger panicked on error: %v", r)
		}
	}()

	// Suppress stderr output during test
	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w

	// Try various invalid paths
	invalidPaths := []string{
		"/nonexistent/path/test.log",
		"",
		"/dev/null/invalid/path.log",
	}

	for _, path := range invalidPaths {
		logger := NewLogger(nil, path, true)
		logger.Info("Test", "message")
		logger.Close()
	}

	w.Close()
	os.Stderr = oldStderr
}

// TestLoggerContinuesAfterError tests that logger can continue logging after file error
func TestLoggerContinuesAfterError(t *testing.T) {
	invalidPath := "/nonexistent_test_path/log.txt"

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logger := NewLogger(nil, invalidPath, true)

	// Log multiple messages
	logger.Info("Project1", "message 1")
	logger.Warn("Project2", "message 2")
	logger.Error("Project3", "message 3")

	logger.Close()

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	buf.ReadFrom(r)
	stderrOutput := buf.String()

	// Verify all messages were logged to stderr
	if !strings.Contains(stderrOutput, "message 1") {
		t.Error("Expected message 1 in output")
	}
	if !strings.Contains(stderrOutput, "message 2") {
		t.Error("Expected message 2 in output")
	}
	if !strings.Contains(stderrOutput, "message 3") {
		t.Error("Expected message 3 in output")
	}
}

// TestLoggerConsoleModeStderr tests that console mode (daemonMode=false) logs to stderr
func TestLoggerConsoleModeStderr(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Create logger in console mode (daemonMode=false)
	logger := NewLogger(nil, "", false)

	logger.Info("ConsoleProject", "console mode message")

	logger.Close()

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	buf.ReadFrom(r)
	stderrOutput := buf.String()

	// Verify message was logged to stderr
	if !strings.Contains(stderrOutput, "console mode message") {
		t.Errorf("Expected console mode message in stderr, got: %s", stderrOutput)
	}
	if !strings.Contains(stderrOutput, "[ConsoleProject]") {
		t.Errorf("Expected project name in stderr, got: %s", stderrOutput)
	}
}

// TestLoggerDaemonModeFile tests that daemon mode (daemonMode=true) logs to file
func TestLoggerDaemonModeFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "daemon.log")

	// Create logger in daemon mode (daemonMode=true)
	logger := NewLogger(nil, logPath, true)

	logger.Info("DaemonProject", "daemon mode message")

	logger.Close()

	// Verify message was logged to file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "daemon mode message") {
		t.Errorf("Expected daemon mode message in log file, got: %s", string(content))
	}
	if !strings.Contains(string(content), "[DaemonProject]") {
		t.Errorf("Expected project name in log file, got: %s", string(content))
	}
}

// TestLoggerConsoleModeIgnoresFilePath tests that console mode ignores file path parameter
func TestLoggerConsoleModeIgnoresFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "should_not_exist.log")

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Create logger in console mode with a file path (should be ignored)
	logger := NewLogger(nil, logPath, false)

	logger.Info("Project", "test message")

	logger.Close()

	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	buf.ReadFrom(r)
	stderrOutput := buf.String()

	// Verify message went to stderr
	if !strings.Contains(stderrOutput, "test message") {
		t.Errorf("Expected message in stderr, got: %s", stderrOutput)
	}

	// Verify file was NOT created (console mode should ignore file path)
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Error("Expected log file to NOT be created in console mode")
	}
}
