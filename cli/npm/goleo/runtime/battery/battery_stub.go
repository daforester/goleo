//go:build !windows && !darwin && !linux

package battery

import (
	"errors"
	"fmt"
	"runtime"
)

func platformGetBatteryInfo() (*BatteryInfo, error) {
	return nil, fmt.Errorf("battery: %w on %s", errors.ErrUnsupported, runtime.GOOS)
}
