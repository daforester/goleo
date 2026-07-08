//go:build windows

package cmd

import (
	"os/exec"
	"strconv"
)

func killProcTree(pid int) {
	if pid <= 0 {
		return
	}
	exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
}
