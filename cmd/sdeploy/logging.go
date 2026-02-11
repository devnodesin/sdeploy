package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LogWriter is an interface that both Logger and BuildLogger implement
type LogWriter interface {
	Info(project, message string)
	Warn(project, message string)
	Error(project, message string)
	Infof(project, format string, args ...interface{})
	Warnf(project, format string, args ...interface{})
	Errorf(project, format string, args ...interface{})
}

// Logger provides thread-safe logging with configurable output
// This is a centralized logger that routes logs to different destinations
type Logger struct {
	mu         sync.Mutex
	writer     io.Writer // for testing or service logs
	file       *os.File
	logPath    string // base directory for logs
	daemonMode bool
}

// BuildLogger handles logging for a specific project build
type BuildLogger struct {
	mu          sync.Mutex
	writer      io.Writer
	file        *os.File
	projectName string
	logDir      string // base directory for log files
	startTime   time.Time
	logPath     string // temporary path without status
	finalPath   string // final path with success/fail status
	daemonMode  bool
}

// NewLogger creates a new logger instance
// Service logs are always written to {log_path}/main.log regardless of mode
// If writer is provided, logs go to that writer (used for testing)
// If logPath is provided, it's used as the base log directory
// If daemonMode is false (console mode), service logs go to both main.log and stderr
// If daemonMode is true (daemon mode), service logs go only to main.log
// Falls back to stderr when file operations fail
func NewLogger(writer io.Writer, logPath string, daemonMode bool) *Logger {
	l := &Logger{
		daemonMode: daemonMode,
	}

	// If writer is provided, use it directly (for testing)
	if writer != nil {
		l.writer = writer
		// Still set logPath for build loggers
		if logPath != "" {
			l.logPath = logPath
		} else {
			l.logPath = Defaults.LogPath
		}
		return l
	}

	// Determine log directory
	if logPath != "" {
		l.logPath = logPath
	} else {
		l.logPath = Defaults.LogPath
	}

	// Ensure log directory exists
	if err := os.MkdirAll(l.logPath, 0755); err != nil {
		reportLogFileError("create directory", l.logPath, err, "0755")
		l.writer = os.Stderr
		return l
	}

	// Open main.log file for service logs (always, regardless of mode)
	mainLogPath := filepath.Join(l.logPath, "main.log")
	file, err := os.OpenFile(mainLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		reportLogFileError("open/create file", mainLogPath, err, "0644")
		l.writer = os.Stderr
		return l
	}

	l.file = file

	// In console mode: logs go to both main.log and stderr
	// In daemon mode: logs go only to main.log
	if !daemonMode {
		// Console mode: use MultiWriter to write to both file and stderr
		l.writer = io.MultiWriter(file, os.Stderr)
	} else {
		// Daemon mode: write only to file
		l.writer = file
	}

	return l
}

// NewBuildLogger creates a logger for a specific project build
// Build logs are always written to a file, even in console mode
// Filename format: {project_name}-{yyyy-mm-dd}-{HHMM}-{status}.log
// Status is set when Close is called
func (l *Logger) NewBuildLogger(projectName string) *BuildLogger {
	bl := &BuildLogger{
		projectName: projectName,
		startTime:   time.Now(),
		daemonMode:  l.daemonMode,
	}

	// Determine log directory
	logDir := l.logPath
	if logDir == "" {
		logDir = Defaults.LogPath
	}

	// Store the log directory for later use
	bl.logDir = logDir

	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		// Fallback to stderr if directory creation fails
		bl.writer = os.Stderr
		fmt.Fprintf(os.Stderr, "[SDeploy] Failed to create build log directory: %v\n", err)
		return bl
	}

	// Create temporary filename (without status)
	// Format: {project_name}-{yyyy-mm-dd}-{HHMM}-pending.log
	// Sanitize project name to prevent nested directories
	sanitizedName := sanitizeProjectName(projectName)
	timestamp := bl.startTime.Format("2006-01-02-1504")
	tempFilename := fmt.Sprintf("%s-%s-pending.log", sanitizedName, timestamp)
	bl.logPath = filepath.Join(logDir, tempFilename)

	// Open the build log file
	file, err := os.OpenFile(bl.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// Fallback to stderr if file creation fails
		bl.writer = os.Stderr
		fmt.Fprintf(os.Stderr, "[SDeploy] Failed to create build log file: %v\n", err)
	} else {
		bl.file = file
		bl.writer = file
	}

	return bl
}

// Close closes the build logger and renames the file with success/fail status
func (bl *BuildLogger) Close(success bool) {
	if bl == nil {
		return
	}
	
	bl.mu.Lock()
	defer bl.mu.Unlock()

	// Close the file first
	if bl.file != nil {
		bl.file.Close()
		bl.file = nil
	}

	// Rename the file to include success/fail status
	if bl.logPath != "" {
		status := "fail"
		if success {
			status = "success"
		}

		// Determine final filename using stored logDir
		// Sanitize project name to prevent nested directories
		sanitizedName := sanitizeProjectName(bl.projectName)
		timestamp := bl.startTime.Format("2006-01-02-1504")
		finalFilename := fmt.Sprintf("%s-%s-%s.log", sanitizedName, timestamp, status)
		bl.finalPath = filepath.Join(bl.logDir, finalFilename)

		// Rename the file
		if err := os.Rename(bl.logPath, bl.finalPath); err != nil {
			fmt.Fprintf(os.Stderr, "[SDeploy] Failed to rename build log file: %v\n", err)
		}
	}
}

// GetFinalPath returns the final path of the build log file after Close is called
func (bl *BuildLogger) GetFinalPath() string {
	if bl == nil {
		return ""
	}
	bl.mu.Lock()
	defer bl.mu.Unlock()
	return bl.finalPath
}

// log writes a log message to the build logger
func (bl *BuildLogger) log(level, project, message string) {
	if bl == nil {
		return
	}
	
	bl.mu.Lock()
	defer bl.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	var logLine string
	if project == "" {
		logLine = fmt.Sprintf("[%s] [%s] %s\n", timestamp, level, message)
	} else {
		logLine = fmt.Sprintf("[%s] [%s] [%s] %s\n", timestamp, level, project, message)
	}
	
	if bl.writer != nil {
		_, _ = bl.writer.Write([]byte(logLine))
	}
}

// Info logs an informational message to the build log
func (bl *BuildLogger) Info(project, message string) {
	bl.log("INFO", project, message)
}

// Warn logs a warning message to the build log
func (bl *BuildLogger) Warn(project, message string) {
	bl.log("WARN", project, message)
}

// Error logs an error message to the build log
func (bl *BuildLogger) Error(project, message string) {
	bl.log("ERROR", project, message)
}

// Infof logs a formatted informational message to the build log
func (bl *BuildLogger) Infof(project, format string, args ...interface{}) {
	bl.Info(project, fmt.Sprintf(format, args...))
}

// Warnf logs a formatted warning message to the build log
func (bl *BuildLogger) Warnf(project, format string, args ...interface{}) {
	bl.Warn(project, fmt.Sprintf(format, args...))
}

// Errorf logs a formatted error message to the build log
func (bl *BuildLogger) Errorf(project, format string, args ...interface{}) {
	bl.Error(project, fmt.Sprintf(format, args...))
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

// sanitizeProjectName replaces path separators in project names with underscores
// This prevents project names like "domain.com/project" from creating nested directories
func sanitizeProjectName(projectName string) string {
	// Replace both forward and back slashes with underscores
	sanitized := strings.ReplaceAll(projectName, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, "\\", "_")
	return sanitized
}

// IsDaemonMode returns whether the logger is in daemon mode
func (l *Logger) IsDaemonMode() bool {
	return l.daemonMode
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
