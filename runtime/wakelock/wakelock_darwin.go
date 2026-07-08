//go:build darwin && !ios

package wakelock

import (
	"fmt"
	"os/exec"
	"sync"
)

var (
	mu   sync.Mutex
	proc *exec.Cmd
)

func platformRequest(typeName string) error {
	mu.Lock()
	defer mu.Unlock()
	if proc != nil {
		return nil // already held
	}

	args := "-i" // prevent idle system sleep
	if typeName != "system" {
		args = "-di" // also keep the display awake
	}
	cmd := exec.Command("caffeinate", args)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("wakelock: caffeinate failed to start: %w", err)
	}
	proc = cmd
	return nil
}

func platformRelease() error {
	mu.Lock()
	defer mu.Unlock()
	if proc == nil {
		return nil
	}
	if err := proc.Process.Kill(); err != nil {
		return fmt.Errorf("wakelock: failed to stop caffeinate: %w", err)
	}
	proc.Wait()
	proc = nil
	return nil
}
