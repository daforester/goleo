//go:build !(android || ios) || goleo_battery

package battery

import "sync"

// BatteryInfo mirrors the web Battery Status API: Level is 0..1, and the
// time fields are seconds with -1 meaning unknown.
type BatteryInfo struct {
	Level           float64 `json:"level"`
	Charging        bool    `json:"charging"`
	ChargingTime    float64 `json:"chargingTime"`
	DischargingTime float64 `json:"dischargingTime"`
}

// Provider is a native battery backend. On mobile the shell registers one
// via SetProvider; on desktop the built-in platform implementation is used
// when no provider is set.
type Provider interface {
	GetBatteryInfo() (*BatteryInfo, error)
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

func GetBatteryInfo() (*BatteryInfo, error) {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p != nil {
		return p.GetBatteryInfo()
	}
	return platformGetBatteryInfo()
}
