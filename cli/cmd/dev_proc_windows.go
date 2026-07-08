//go:build windows

package cmd

import (
	"fmt"
	"os/exec"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

func killProcTree(pid int) {
	if pid <= 0 {
		return
	}
	exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
}

// devJob is a Windows Job Object that dev-session child processes (the Go
// backend, the Vite server) are assigned to via bindProcessLifetime. It is
// intentionally never closed explicitly: with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// set, Windows kills every process still in the job the instant this goleo
// process's last handle to it closes — which happens automatically on ANY
// exit path (clean return, panic, or being force-killed from Task Manager
// or a closed terminal window). killProcTree above only runs on the clean
// shutdown path; this is the backstop for every other path, so `go run`'s
// spawned backend.exe can't survive as an orphan silently answering dev
// requests with stale code after a bad shutdown.
var devJob windows.Handle

func bindProcessLifetime(cmd *exec.Cmd) error {
	if devJob == 0 {
		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return fmt.Errorf("CreateJobObject: %w", err)
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		if _, err := windows.SetInformationJobObject(
			job,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		); err != nil {
			windows.CloseHandle(job)
			return fmt.Errorf("SetInformationJobObject: %w", err)
		}
		devJob = job
	}

	procHandle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_SET_QUOTA, false, uint32(cmd.Process.Pid))
	if err != nil {
		return fmt.Errorf("OpenProcess: %w", err)
	}
	defer windows.CloseHandle(procHandle)

	if err := windows.AssignProcessToJobObject(devJob, procHandle); err != nil {
		return fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return nil
}
