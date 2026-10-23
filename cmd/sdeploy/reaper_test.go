package main

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestReapChildrenReapsExitedUnwaitedChild verifies that the reaper collects
// a child that exited without anyone calling Wait, which is what happens to
// descendants reparented to sdeploy when it runs as PID 1.
func TestReapChildrenReapsExitedUnwaitedChild(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("zombie-state observation relies on /proc, available on Linux only")
	}
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start child: %v", err)
	}
	pid := cmd.Process.Pid

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && procState(pid) != "Z" {
		time.Sleep(10 * time.Millisecond)
	}
	if got := procState(pid); got != "Z" {
		t.Fatalf("child %d did not become a zombie, state %q", pid, got)
	}

	if n := reapChildren(); n < 1 {
		t.Fatalf("expected reaper to reap at least 1 child, got %d", n)
	}

	var status syscall.WaitStatus
	if _, err := syscall.Wait4(pid, &status, syscall.WNOHANG, nil); err != syscall.ECHILD {
		t.Fatalf("expected child %d to be reaped (ECHILD), got err=%v", pid, err)
	}
}

// TestReapChildrenWaitsForNoChild verifies the reaper returns promptly when
// there is nothing to collect.
func TestReapChildrenWaitsForNoChild(t *testing.T) {
	done := make(chan int, 1)
	go func() {
		done <- reapChildren()
	}()

	select {
	case n := <-done:
		if n != 0 {
			t.Errorf("expected 0 reaped children, got %d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reapChildren did not return")
	}
}

// TestReaperDoesNotStealManagedChildren verifies that reaping does not
// consume the exit status of commands run through the helpers.
func TestReaperDoesNotStealManagedChildren(t *testing.T) {
	stop := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			select {
			case <-stop:
				return
			default:
				reapChildren()
				time.Sleep(time.Millisecond)
			}
		}
	}()

	for i := 0; i < 20; i++ {
		if err := runCommand(exec.Command("true")); err != nil {
			close(stop)
			<-finished
			t.Fatalf("managed command %d failed: %v", i, err)
		}
	}

	close(stop)
	<-finished
}

// TestRunCombinedOutput captures output through the coordinated helper.
func TestRunCombinedOutput(t *testing.T) {
	out, err := runCombinedOutput(exec.Command("sh", "-c", "printf hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "hello" {
		t.Errorf("expected %q, got %q", "hello", string(out))
	}
}

// procState returns the single-letter state of a process from /proc.
func procState(pid int) string {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return ""
	}
	return fields[2]
}
