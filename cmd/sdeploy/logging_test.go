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
	logger := NewLogger(&buf, "")

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
	logger := NewLogger(&buf, "")

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

	logger := NewLogger(nil, logPath)
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
	logger := NewLogger(&buf, "")

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
	logger := NewLogger(&buf, "")

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
	logger := NewLogger(nil, logPath)
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
	logger := NewLogger(&buf, "")

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
	logger := NewLogger(&buf, "")

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

	logger := NewLogger(nil, logPath)
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
