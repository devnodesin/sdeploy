package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// childReapInterval is how often orphaned children are collected.
const childReapInterval = 2 * time.Second

// reapWaitDelay bounds how long a managed command's Wait waits for inherited
// output pipes to close after its process has exited, so a descendant that
// escapes the process group (e.g. via setsid) and keeps a pipe open cannot
// block Wait forever, which would otherwise pin this project's lock and
// goroutine indefinitely.
const reapWaitDelay = 5 * time.Second

// managedChildren holds the PIDs of processes sdeploy is actively waiting on
// via exec.Cmd. The reaper only reaps reparented orphans (descendants whose
// parent shell already exited) whose PIDs are not in this set. Excluding
// managed PIDs guarantees the reaper can never consume a status that
// exec.Cmd.Wait is waiting for.
var (
	managedMu       sync.Mutex
	managedChildren = map[int]struct{}{}
)

func registerChild(pid int) {
	managedMu.Lock()
	managedChildren[pid] = struct{}{}
	managedMu.Unlock()
}

func unregisterChild(pid int) {
	managedMu.Lock()
	delete(managedChildren, pid)
	managedMu.Unlock()
}

func isManagedChild(pid int) bool {
	managedMu.Lock()
	_, ok := managedChildren[pid]
	managedMu.Unlock()
	return ok
}

// runCombinedOutput runs cmd, captures combined output, and waits while marking
// the child as managed so the orphan reaper cannot reap it first.
func runCombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	cmd.WaitDelay = reapWaitDelay
	registerChild(cmd.Process.Pid)
	defer unregisterChild(cmd.Process.Pid)

	// stdout and stderr are copied from separate goroutines, so the shared
	// buffer must be protected against concurrent writes.
	var buf bytes.Buffer
	var bufMu sync.Mutex
	writeSafe := func(w io.Writer) io.Writer {
		return &lockedWriter{mu: &bufMu, w: w}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(writeSafe(&buf), stdout) }()
	go func() { defer wg.Done(); io.Copy(writeSafe(&buf), stderr) }()
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return buf.Bytes(), err
	}
	return buf.Bytes(), nil
}

// lockedWriter serializes writes to an underlying io.Writer so it can be shared
// safely by concurrent copy goroutines.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// runCommand runs cmd while marking the child as managed.
func runCommand(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	cmd.WaitDelay = reapWaitDelay
	registerChild(cmd.Process.Pid)
	defer unregisterChild(cmd.Process.Pid)
	return cmd.Wait()
}

// reapChildren collects descendant processes that were reparented to sdeploy
// (it runs as PID 1 in the container) and have exited without a waiter of
// their own. It enumerates direct children via /proc and, for any that are not
// managed by sdeploy, reaps them. Reaping specific orphan PIDs — never a
// process-wide wait4(-1) — means an in-flight exec.Cmd is never disturbed, and
// it cannot race with Go's own exec.Cmd.Wait.
func reapChildren() int {
	self := os.Getpid()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	reaped := 0
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		ppid, ok := childParentPID(e.Name())
		if !ok || ppid != self {
			continue
		}
		if isManagedChild(pid) {
			continue
		}
		var status syscall.WaitStatus
		wpid, werr := syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
		if wpid == pid && werr == nil {
			reaped++
		}
	}
	return reaped
}

// childParentPID reads the PPID field from /proc/<pid>/stat. The comm field may
// contain spaces or parentheses, but the PPID is always the 4th
// whitespace-separated field.
func childParentPID(name string) (int, bool) {
	data, err := os.ReadFile("/proc/" + name + "/stat")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 4 {
		return 0, false
	}
	ppid, err := strconv.Atoi(fields[3])
	if err != nil {
		return 0, false
	}
	return ppid, true
}

// startChildReaper periodically reaps orphaned child processes. It only runs
// when sdeploy is PID 1 (for example inside the Docker image); on every other
// host the OS reaps its own children, so the reaper is skipped rather than
// adding per-tick overhead or contention.
func startChildReaper(logger *Logger) {
	if os.Getpid() != 1 {
		return
	}
	go func() {
		ticker := time.NewTicker(childReapInterval)
		defer ticker.Stop()
		for range ticker.C {
			if n := reapChildren(); n > 0 && logger != nil {
				logger.Infof("", "Reaped %d exited child process(es)", n)
			}
		}
	}()
}
