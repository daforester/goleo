//go:build windows

package cmd

import (
	"os/exec"
	"syscall"
)

// sdkToolCommand builds the command to run a cmdline-tools launcher (sdkmanager,
// avdmanager) on Windows, where the launcher is a .bat that must be run through
// cmd.exe. The raw command line is set via SysProcAttr.CmdLine so each argument
// stays quoted intact (see windowsSdkToolCmdLine); Go's default argument
// escaping would not quote specs like "platforms;android-34", which cmd/batch
// would then split on ';'.
func sdkToolCommand(tool string, args []string) *exec.Cmd {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: windowsSdkToolCmdLine(tool, args)}
	return cmd
}
