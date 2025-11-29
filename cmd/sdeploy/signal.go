package main

import (
	"os"
	"syscall"
)

// getShutdownSignals returns the signals to listen for graceful shutdown
func getShutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}
