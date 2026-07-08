//go:build !windows && !darwin && !android

package geolocation

import (
	"errors"
	"fmt"
	"runtime"
)

// No portable location source on this platform (Linux would need a GeoClue
// D-Bus client); the frontend falls back to navigator.geolocation.
func platformGetCurrentPosition(opts PositionOptions) (*Position, error) {
	return nil, fmt.Errorf("geolocation: %w on %s", errors.ErrUnsupported, runtime.GOOS)
}
