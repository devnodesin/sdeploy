package main

import (
	"syscall"
	"testing"
)

// TestGetShutdownSignals tests that getShutdownSignals returns expected signals
func TestGetShutdownSignals(t *testing.T) {
	signals := getShutdownSignals()

	if len(signals) == 0 {
		t.Error("Expected at least one shutdown signal")
	}

	// Verify SIGINT and SIGTERM are included (Unix shutdown signals)
	hasInt := false
	hasTerm := false
	for _, sig := range signals {
		if sig == syscall.SIGINT {
			hasInt = true
		}
		if sig == syscall.SIGTERM {
			hasTerm = true
		}
	}

	if !hasInt {
		t.Error("Expected SIGINT to be in shutdown signals")
	}
	if !hasTerm {
		t.Error("Expected SIGTERM to be in shutdown signals")
	}
}

// TestGetShutdownSignalsCount tests that exactly two signals are returned
func TestGetShutdownSignalsCount(t *testing.T) {
	signals := getShutdownSignals()

	if len(signals) != 2 {
		t.Errorf("Expected 2 shutdown signals, got %d", len(signals))
	}
}
