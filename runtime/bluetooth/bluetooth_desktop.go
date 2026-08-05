//go:build !android && !ios

package bluetooth

import (
	"errors"
	"fmt"
)

var errUnsupported = fmt.Errorf("bluetooth: %w on desktop (use the Web Bluetooth fallback)", errors.ErrUnsupported)

func platformRequestDevice(filters map[string]any) (*BLEDevice, error) {
	return nil, errUnsupported
}

func platformConnect(deviceID string) error {
	return errUnsupported
}

func platformDisconnect(deviceID string) error {
	return errUnsupported
}

func platformRead(deviceID, service, characteristic string) ([]byte, error) {
	return nil, errUnsupported
}

func platformWrite(deviceID, service, characteristic string, data []byte) error {
	return errUnsupported
}
