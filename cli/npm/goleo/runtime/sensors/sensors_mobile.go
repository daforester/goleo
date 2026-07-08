//go:build (android || ios) && goleo_sensors

package sensors

import "errors"

var errNoProvider = errors.New("sensors: no native provider registered: the mobile shell must call SetProvider at startup")

func platformStartSensor(sensorType string) error {
	return errNoProvider
}

func platformStopSensor(sensorType string) error {
	return errNoProvider
}
