//go:build !(android || ios) || goleo_geolocation

package geolocation

import "sync"

type Position struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  float64 `json:"accuracy,omitempty"`
}

type PositionOptions struct {
	EnableHighAccuracy bool `json:"enableHighAccuracy,omitempty"`
	Timeout            int  `json:"timeout,omitempty"`    // ms
	MaximumAge         int  `json:"maximumAge,omitempty"` // ms
}

// Provider is a native geolocation backend. On mobile the shell registers
// one via SetProvider (FusedLocationProvider / CLLocationManager); on
// desktop the built-in platform implementation is used when no provider is
// set.
type Provider interface {
	GetCurrentPosition(opts PositionOptions) (*Position, error)
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

func GetCurrentPosition(opts PositionOptions) (*Position, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p != nil {
		return p.GetCurrentPosition(opts)
	}
	return platformGetCurrentPosition(opts)
}
