//go:build !android && !ios

package sensors

import (
	"errors"
	"fmt"
)

func platformStartSensor(sensorType string) error {
	return fmt.Errorf("sensors: %w on desktop", errors.ErrUnsupported)
}

func platformStopSensor(sensorType string) error {
	return fmt.Errorf("sensors: %w on desktop", errors.ErrUnsupported)
}
