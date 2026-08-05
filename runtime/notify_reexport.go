package runtime

import "github.com/daforester/goleo/runtime/notify"

type NativeNotifier = notify.Notifier

func SetNativeNotifier(n NativeNotifier) {
	notify.SetNotifier(n)
}

func Notify(title, body string) error {
	return notify.Notify(title, body)
}

func NotificationPermissionGranted() bool {
	return notify.PermissionGranted()
}

func RequestNotificationPermission() string {
	return notify.RequestPermission()
}
