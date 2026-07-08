//go:build !windows && !darwin && !linux

package wakelock

import (
	"errors"
	"fmt"
	"runtime"
)

func platformRequest(typeName string) error {
	return fmt.Errorf("wakelock: %w on %s", errors.ErrUnsupported, runtime.GOOS)
}

func platformRelease() error {
	return nil
}
