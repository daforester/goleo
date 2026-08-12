//go:build !windows

package cmd

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestNewProcessGroupIsolatesChild verifies that a child started via
// newProcessGroup becomes its own process-group leader, so killProcTree only
// reaps the child — not goleo (here, the test process). If newProcessGroup were
// broken, the child would share the test's group and killProcTree would SIGKILL
// the test runner; the pre-kill assertion below guards against that by refusing
// to call killProcTree unless the child is confirmed to be in its own group.
func TestNewProcessGroupIsolatesChild(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	newProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	pid := cmd.Process.Pid
	defer func() { _ = cmd.Process.Kill() }() // safety net

	// The child must lead its own group (pgid == pid) and that group must be
	// different from the test process's own group. Only then is it safe to call
	// killProcTree, which signals the whole group.
	childPgid, err := syscall.Getpgid(pid)
	if err != nil {
		t.Fatalf("getpgid(child): %v", err)
	}
	if childPgid != pid {
		t.Fatalf("child pgid = %d, want it to lead its own group (pid %d)", childPgid, pid)
	}
	if selfPgid, _ := syscall.Getpgid(0); childPgid == selfPgid {
		t.Fatalf("child shares the test process group (%d); killProcTree would kill the runner", selfPgid)
	}

	// Safe now: this kills only the child's group.
	killProcTree(pid)

	// The child must actually die...
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	select {
	case err := <-waited:
		if err == nil {
			t.Fatalf("child exited cleanly; expected it to be killed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child was not killed within timeout")
	}

	// ...and reaching here proves the test process (the "parent") survived.
}
