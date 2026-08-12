//go:build !windows && !darwin && (!linux || android)

package autostart

import (
	"errors"
	"fmt"
)

// Mobile/wasm have no user-login autostart concept.

func platformEnable(appName, exePath string) error {
	return fmt.Errorf("autostart: %w", errors.ErrUnsupported)
}

func platformDisable(appName string) error {
	return fmt.Errorf("autostart: %w", errors.ErrUnsupported)
}

func platformIsEnabled(appName string) (bool, error) {
	return false, nil
}
