//go:build !(android || ios) || goleo_push

package push

import "sync"

type PushSubscription struct {
	Endpoint string            `json:"endpoint"`
	Keys     map[string]string `json:"keys"`
}

// Provider is a native push backend. On mobile the shell registers one via
// SetProvider (Firebase Cloud Messaging on Android, APNs on iOS). Desktop
// operating systems have no unified push service; deliver server events over
// the app's own WebSocket connection instead.
type Provider interface {
	Subscribe(serverKey string) (*PushSubscription, error)
	Unsubscribe() error
	GetSubscription() (*PushSubscription, error)
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

func Subscribe(serverKey string) (*PushSubscription, error) {
	if p := getProvider(); p != nil {
		return p.Subscribe(serverKey)
	}
	return platformSubscribe(serverKey)
}

func Unsubscribe() error {
	if p := getProvider(); p != nil {
		return p.Unsubscribe()
	}
	return platformUnsubscribe()
}

func GetSubscription() (*PushSubscription, error) {
	if p := getProvider(); p != nil {
		return p.GetSubscription()
	}
	return platformGetSubscription()
}
