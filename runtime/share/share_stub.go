//go:build !windows && !darwin && !linux

package share

import (
	"errors"
	"fmt"
	"runtime"
)

func platformShare(data *ShareData) error {
	return fmt.Errorf("share: %w on %s", errors.ErrUnsupported, runtime.GOOS)
}
