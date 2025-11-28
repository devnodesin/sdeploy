//go:build windows
// +build windows

package main

import (
	"os"
)

// getShutdownSignals returns the signals to listen for graceful shutdown (Windows)
func getShutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
