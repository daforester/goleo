//go:build !windows

package cmd

import (
	"os/exec"
	"strconv"
	"syscall"
)

func killProcTree(pid int) {
	if pid <= 0 {
		return
	}
	pgid, err := syscall.Getpgid(pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGKILL)
	}
	syscall.Kill(pid, syscall.SIGKILL)
}

// newProcessGroup makes cmd start as the leader of its own process group. This
// is required before killProcTree(cmd.Pid) is safe: killProcTree signals the
// whole process group, so without a dedicated group the child shares goleo's
// (and npm's) group and killing it would take down goleo itself. As group
// leader, the child's pgid equals its pid, so Kill(-pid) reaps only the child
// and its descendants.
func newProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// bindProcessLifetime is a no-op on non-Windows platforms. The equivalent
// orphan risk exists here too (a SIGKILL'd goleo process can't run its own
// cleanup), but killing process groups reliably requires each child to be
// started with Setpgid, which dev.go doesn't currently do — left for a
// follow-up rather than bundled into the Windows fix.
func bindProcessLifetime(cmd *exec.Cmd) error {
	return nil
}

func init() {
	_ = strconv.Itoa
}
