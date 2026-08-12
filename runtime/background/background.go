//go:build !(android || ios) || goleo_background

package background

import "sync"

// Provider is a native background-work backend. On mobile the shell
// registers one via SetProvider (WorkManager on Android, BGTaskScheduler on
// iOS). On desktop the app process is always running, so background sync is
// unnecessary; registration errs with ErrUnsupported.
type Provider interface {
	RegisterSync(tag string) error
	GetPermission() bool
	RequestPermission() error
}

var (
	providerMu sync.RWMutex
	provider   Provider
)

func SetProvider(p Provider) {
	providerMu.Lock()
	defer providerMu.Unlock()
	provider = p
}

func getProvider() Provider {
	providerMu.RLock()
	defer providerMu.RUnlock()
	return provider
}

func RegisterSync(tag string) error {
	if p := getProvider(); p != nil {
		return p.RegisterSync(tag)
	}
	return platformRegisterSync(tag)
}

func GetPermission() bool {
	if p := getProvider(); p != nil {
		return p.GetPermission()
	}
	return platformGetPermission()
}

func RequestPermission() error {
	if p := getProvider(); p != nil {
		return p.RequestPermission()
	}
	return platformRequestPermission()
}
