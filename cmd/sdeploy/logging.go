package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger provides thread-safe logging with configurable output
type Logger struct {
	mu       sync.Mutex
	writer   io.Writer
	file     *os.File
	filePath string
}

// NewLogger creates a new logger instance
// If writer is provided, logs go to that writer (used for testing)
// If filePath is provided, logs go to file (appending mode)
// If both are nil/empty, logs go to stdout
func NewLogger(writer io.Writer, filePath ...string) *Logger {
	l := &Logger{}

	// If writer is provided, use it directly (for testing)
	if writer != nil {
		l.writer = writer
		return l
	}

	// Determine log file path
	logPath := "/var/log/sdeploy.log"
	if len(filePath) > 0 && filePath[0] != "" {
		logPath = filePath[0]
	}
	l.filePath = logPath

	// Ensure parent directory exists
	if err := ensureParentDir(logPath); err != nil {
		reportLogFileError("create directory", filepath.Dir(logPath), err, "0755")
		l.writer = os.Stderr
		return l
	}

	// Open log file
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		reportLogFileError("open/create file", logPath, err, "0644")
		l.writer = os.Stderr
	} else {
		l.file = file
		l.writer = file
	}
	return l
}

// reportLogFileError outputs a detailed error message to stderr when log file operations fail
func reportLogFileError(operation, path string, err error, attemptedPerms string) {
	fmt.Fprintf(os.Stderr, "\n[SDeploy] Log file error: failed to %s\n", operation)
	fmt.Fprintf(os.Stderr, "  Path: %s\n", path)
	fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
	fmt.Fprintf(os.Stderr, "  Attempted permissions: %s\n", attemptedPerms)

	// Provide specific guidance based on error type
	if errors.Is(err, os.ErrPermission) {
		fmt.Fprintf(os.Stderr, "  Cause: Permission denied\n")
		reportFilePermissions(path)
		fmt.Fprintf(os.Stderr, "  Suggestions:\n")
		fmt.Fprintf(os.Stderr, "    - Run sdeploy as root or with sudo\n")
		fmt.Fprintf(os.Stderr, "    - Change ownership: sudo chown $USER %s\n", filepath.Dir(path))
		fmt.Fprintf(os.Stderr, "    - Change permissions: sudo chmod 755 %s\n", filepath.Dir(path))
	} else if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "  Cause: Path does not exist\n")
		fmt.Fprintf(os.Stderr, "  Suggestions:\n")
		fmt.Fprintf(os.Stderr, "    - Create directory: sudo mkdir -p %s\n", filepath.Dir(path))
		fmt.Fprintf(os.Stderr, "    - Set permissions: sudo chmod 755 %s\n", filepath.Dir(path))
	} else {
		fmt.Fprintf(os.Stderr, "  Suggestions:\n")
		fmt.Fprintf(os.Stderr, "    - Verify the path is valid and accessible\n")
		fmt.Fprintf(os.Stderr, "    - Check disk space and filesystem status\n")
	}

	fmt.Fprintf(os.Stderr, "  Fallback: Logging to console (stderr)\n\n")
}

// reportFilePermissions attempts to report current file/directory permissions
func reportFilePermissions(path string) {
	// Try the path itself first, then parent directory
	pathsToCheck := []string{path, filepath.Dir(path)}

	for _, p := range pathsToCheck {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}

		fmt.Fprintf(os.Stderr, "  Current permissions for %s:\n", p)
		fmt.Fprintf(os.Stderr, "    Mode: %s\n", info.Mode().String())

		// Get owner/group info (platform-specific, handled via helper)
		if ownerInfo := getFileOwnerInfo(info); ownerInfo != "" {
			fmt.Fprintf(os.Stderr, "    Owner: %s\n", ownerInfo)
		}
		return
	}
}

// ensureParentDir creates the parent directory of the given file path if it doesn't exist
func ensureParentDir(filePath string) error {
	dir := filepath.Dir(filePath)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

// Close closes the underlying file if one was opened
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

// log writes a log message with the specified level
func (l *Logger) log(level, project, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	var logLine string
	if project == "" {
		// No project specified, use simpler format without empty brackets
		logLine = fmt.Sprintf("[%s] [%s] %s\n", timestamp, level, message)
	} else {
		logLine = fmt.Sprintf("[%s] [%s] [%s] %s\n", timestamp, level, project, message)
	}
	_, _ = l.writer.Write([]byte(logLine))
}

// Info logs an informational message
func (l *Logger) Info(project, message string) {
	l.log("INFO", project, message)
}

// Warn logs a warning message
func (l *Logger) Warn(project, message string) {
	l.log("WARN", project, message)
}

// Error logs an error message
func (l *Logger) Error(project, message string) {
	l.log("ERROR", project, message)
}

// Infof logs a formatted informational message
func (l *Logger) Infof(project, format string, args ...interface{}) {
	l.Info(project, fmt.Sprintf(format, args...))
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(project, format string, args ...interface{}) {
	l.Warn(project, fmt.Sprintf(format, args...))
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(project, format string, args ...interface{}) {
	l.Error(project, fmt.Sprintf(format, args...))
}
