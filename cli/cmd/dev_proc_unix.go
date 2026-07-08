//go:build !windows

package cmd

import (
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

func init() {
	_ = strconv.Itoa
}
