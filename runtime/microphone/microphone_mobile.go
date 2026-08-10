//go:build (android || ios) && goleo_microphone

package microphone

import "errors"

var errNoProvider = errors.New("microphone: no native provider registered: the mobile shell must call SetProvider at startup")

func platformPermissionGranted() (bool, error) {
	return false, errNoProvider
}

func platformRequestPermission() (string, error) {
	return "", errNoProvider
}
