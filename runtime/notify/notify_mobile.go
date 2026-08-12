//go:build android || ios

package notify

import "errors"

func platformNotify(title, body string) error {
	return errors.New("no native notifier registered: the mobile shell must call SetNotifier at startup")
}

func platformPermissionGranted() bool {
	return false
}

func platformRequestPermission() string {
	return "denied"
}
