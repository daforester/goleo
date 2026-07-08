//go:build !windows && !darwin && !linux && !android && !ios

package notify

import "errors"

func platformNotify(title, body string) error {
	return errors.New("notifications not supported on this platform")
}

func platformPermissionGranted() bool {
	return false
}

func platformRequestPermission() string {
	return "denied"
}
