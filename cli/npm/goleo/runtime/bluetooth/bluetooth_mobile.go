//go:build (android || ios) && goleo_ble

package bluetooth

import "errors"

var errNoProvider = errors.New("bluetooth: no native provider registered: the mobile shell must call SetProvider at startup")

func platformRequestDevice(filters map[string]any) (*BLEDevice, error) {
	return nil, errNoProvider
}

func platformConnect(deviceID string) error {
	return errNoProvider
}

func platformDisconnect(deviceID string) error {
	return errNoProvider
}

func platformRead(deviceID, service, characteristic string) ([]byte, error) {
	return nil, errNoProvider
}

func platformWrite(deviceID, service, characteristic string, data []byte) error {
	return errNoProvider
}
