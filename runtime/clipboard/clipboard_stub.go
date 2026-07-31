//go:build !windows && !darwin && !linux

package clipboard

import (
	"errors"
	"fmt"
	"runtime"
)

func platformReadText() (string, error) {
	return "", fmt.Errorf("clipboard: %w on %s", errors.ErrUnsupported, runtime.GOOS)
}

func platformWriteText(string) error {
	return fmt.Errorf("clipboard: %w on %s", errors.ErrUnsupported, runtime.GOOS)
}
