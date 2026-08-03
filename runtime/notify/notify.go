package notify

import (
	"errors"
	"sync"
)

// ErrPermissionNotGranted is returned by Notify when a registered provider
// cannot post because the OS has not granted notification permission.
//
// Providers deliver through a void platform call — Android's
// NotificationManagerCompat.notify, for example — so a dropped notification is
// invisible to Go. The Android shell's Notifier.Show returns early when the
// permission is missing (correctly: posting would be discarded anyway), which
// left Notify returning nil for a notification the user never saw, and every
// caller reporting success for a no-op. Checking the permission first is the
// only way to tell the two apart.
var ErrPermissionNotGranted = errors.New("notification permission not granted")

type Notifier interface {
	Show(title, body string) error
	PermissionGranted() bool
	RequestPermission() string
}

var (
	mu          sync.RWMutex
	notifier    Notifier
)

func SetNotifier(n Notifier) {
	mu.Lock()
	defer mu.Unlock()
	notifier = n
}

func getNotifier() Notifier {
	mu.RLock()
	defer mu.RUnlock()
	return notifier
}

func Notify(title, body string) error {
	if title == "" {
		title = "Goleo"
	}
	if n := getNotifier(); n != nil {
		// Desktop providers report granted unconditionally, so this only bites
		// where it must: a mobile shell whose runtime permission is missing.
		if !n.PermissionGranted() {
			return ErrPermissionNotGranted
		}
		return n.Show(title, body)
	}
	return platformNotify(title, body)
}

func PermissionGranted() bool {
	if n := getNotifier(); n != nil {
		return n.PermissionGranted()
	}
	return platformPermissionGranted()
}

func RequestPermission() string {
	if n := getNotifier(); n != nil {
		return n.RequestPermission()
	}
	return platformRequestPermission()
}
