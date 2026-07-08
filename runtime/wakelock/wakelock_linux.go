//go:build linux && !android

package wakelock

import (
	"errors"
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

	bin, err := exec.LookPath("systemd-inhibit")
	if err != nil {
		return fmt.Errorf("wakelock: systemd-inhibit not found: %w", errors.ErrUnsupported)
	}
	what := "sleep"
	if typeName != "system" {
		what = "idle:sleep"
	}
	// The inhibitor lives as long as the wrapped process; killing it on
	// Release drops the lock.
	cmd := exec.Command(bin,
		"--what="+what, "--who=goleo", "--why=application wake lock",
		"--mode=block", "sleep", "infinity")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("wakelock: systemd-inhibit failed to start: %w", err)
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
		return fmt.Errorf("wakelock: failed to stop systemd-inhibit: %w", err)
	}
	proc.Wait()
	proc = nil
	return nil
}
