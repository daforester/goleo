//go:build android || ios

package runtime

import "errors"

// On mobile the OS notification service is only reachable from the native
// shell (Android Activity / iOS AppDelegate), which must register a
// NativeNotifier via SetNativeNotifier through the gomobile bindings.

func platformNotify(title, body string) error {
	return errors.New("no native notifier registered: the mobile shell must call SetNotifier at startup")
}

func platformNotificationPermissionGranted() bool {
	return false
}

func platformRequestNotificationPermission() string {
	return "default"
}
