//go:build !(android || ios) || goleo_wakelock

package wakelock

import "sync"

// Provider is a native wake-lock backend. On mobile the shell registers one
// via SetProvider (e.g. FLAG_KEEP_SCREEN_ON / isIdleTimerDisabled); on
// desktop the built-in platform implementation is used when no provider is
// set.
type Provider interface {
	// Request acquires a wake lock. typeName is "screen" (keep the display
	// on, the web WakeLock API semantic) or "system" (prevent sleep only).
	Request(typeName string) error
	Release() error
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

func Request(typeName string) error {
	if typeName == "" {
		typeName = "screen"
	}
	if p := getProvider(); p != nil {
		return p.Request(typeName)
	}
	return platformRequest(typeName)
}

func Release() error {
	if p := getProvider(); p != nil {
		return p.Release()
	}
	return platformRelease()
}
