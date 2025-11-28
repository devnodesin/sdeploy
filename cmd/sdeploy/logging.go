package main

import (
	"fmt"
	"io"
	"os"
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
// If writer is provided, logs go to that writer
// If filePath is provided, logs go to file (appending mode)
// If both are nil/empty, logs go to stdout
func NewLogger(writer io.Writer, filePath string) *Logger {
	l := &Logger{
		filePath: filePath,
	}

	if filePath != "" {
		// Open file in append mode
		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			// Fall back to stdout on error
			l.writer = os.Stdout
		} else {
			l.file = file
			l.writer = file
		}
	} else if writer != nil {
		l.writer = writer
	} else {
		l.writer = os.Stdout
	}

	return l
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
	logLine := fmt.Sprintf("[%s] [%s] [%s] %s\n", timestamp, level, project, message)
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
