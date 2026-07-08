//go:build !windows

package cmd

import "os/exec"

// sdkToolCommand builds the command to run a cmdline-tools launcher (sdkmanager,
// avdmanager) with the given arguments. On Unix the launcher is a shell script
// executed directly; arguments are passed individually, so no shell or quoting
// is involved.
func sdkToolCommand(tool string, args []string) *exec.Cmd {
	return exec.Command(tool, args...)
}
