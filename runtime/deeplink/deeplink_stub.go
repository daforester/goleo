//go:build !windows && !darwin && (!linux || android)

package deeplink

import (
	"errors"
	"fmt"
)

func platformRegister(scheme, appName, exePath string) error {
	return fmt.Errorf("deeplink: %w", errors.ErrUnsupported)
}
