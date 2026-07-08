//go:build (android || ios) && goleo_background

package background

import "errors"

var errNoProvider = errors.New("background: no native provider registered: the mobile shell must call SetProvider at startup")

func platformRegisterSync(tag string) error {
	return errNoProvider
}

func platformGetPermission() bool {
	return false
}

func platformRequestPermission() error {
	return errNoProvider
}
