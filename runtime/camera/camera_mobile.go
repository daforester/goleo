//go:build (android || ios) && goleo_camera

package camera

import "errors"

var errNoProvider = errors.New("camera: no native provider registered: the mobile shell must call SetProvider at startup")

func platformCapturePhoto() (*PhotoData, error) {
	return nil, errNoProvider
}

func platformStartStream(opts map[string]any) error {
	return errNoProvider
}

func platformStopStream() error {
	return errNoProvider
}
