//go:build !windows

package main

import (
	"os"
	"syscall"
)

// getShutdownSignals returns the signals to listen for graceful shutdown (Unix)
func getShutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}
