package notify

import "sync"

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
