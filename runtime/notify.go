package runtime

import "sync"

// Notification is a system notification request.
type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// NativeNotifier delivers notifications through a platform shell.
// On mobile, the host app (Android Activity / iOS AppDelegate) registers an
// implementation via the generated gomobile bindings so that Notify reaches
// the OS notification service.
type NativeNotifier interface {
	Show(title, body string) error
	PermissionGranted() bool
	// RequestPermission asks the OS for notification permission and returns
	// the resulting state: "granted", "denied" or "default" (undetermined /
	// request still pending).
	RequestPermission() string
}

var (
	notifierMu     sync.RWMutex
	nativeNotifier NativeNotifier
)

// SetNativeNotifier registers the platform notification backend. It replaces
// the built-in desktop implementation; mobile shells must call it before
// notifications can be delivered.
func SetNativeNotifier(n NativeNotifier) {
	notifierMu.Lock()
	defer notifierMu.Unlock()
	nativeNotifier = n
}

func getNativeNotifier() NativeNotifier {
	notifierMu.RLock()
	defer notifierMu.RUnlock()
	return nativeNotifier
}

// Notify shows a system notification. On desktop it uses the OS-native
// mechanism directly; on mobile it delegates to the registered NativeNotifier.
func Notify(title, body string) error {
	if title == "" {
		title = "Goleo"
	}
	if n := getNativeNotifier(); n != nil {
		return n.Show(title, body)
	}
	return platformNotify(title, body)
}

// NotificationPermissionGranted reports whether the app may post notifications.
func NotificationPermissionGranted() bool {
	if n := getNativeNotifier(); n != nil {
		return n.PermissionGranted()
	}
	return platformNotificationPermissionGranted()
}

// RequestNotificationPermission asks the OS for permission to post
// notifications. Returns "granted", "denied" or "default".
func RequestNotificationPermission() string {
	if n := getNativeNotifier(); n != nil {
		return n.RequestPermission()
	}
	return platformRequestNotificationPermission()
}
