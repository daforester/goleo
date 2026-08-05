//go:build windows

package wakelock

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
)

const (
	esContinuous      = 0x80000000
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
)

// SetThreadExecutionState is per-OS-thread, so the lock is held by a
// dedicated goroutine pinned to one thread until Release.
var (
	mu        sync.Mutex
	releaseCh chan struct{}
	doneCh    chan struct{}
)

func platformRequest(typeName string) error {
	mu.Lock()
	defer mu.Unlock()
	if releaseCh != nil {
		return nil // already held
	}

	flags := uintptr(esContinuous | esSystemRequired)
	if typeName != "system" {
		flags |= esDisplayRequired
	}

	rel := make(chan struct{})
	done := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		r, _, callErr := procSetThreadExecutionState.Call(flags)
		if r == 0 {
			errCh <- fmt.Errorf("wakelock: SetThreadExecutionState failed: %w", callErr)
			return
		}
		errCh <- nil
		<-rel
		procSetThreadExecutionState.Call(uintptr(esContinuous))
		close(done)
		// Goroutine exits without UnlockOSThread so the wedged thread is
		// discarded rather than returned to the scheduler pool.
	}()
	if err := <-errCh; err != nil {
		return err
	}
	releaseCh, doneCh = rel, done
	return nil
}

func platformRelease() error {
	mu.Lock()
	defer mu.Unlock()
	if releaseCh == nil {
		return nil
	}
	close(releaseCh)
	<-doneCh
	releaseCh, doneCh = nil, nil
	return nil
}
