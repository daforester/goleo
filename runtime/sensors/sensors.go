//go:build !(android || ios) || goleo_sensors

package sensors

import "sync"

type SensorData struct {
	Type      string  `json:"type"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Z         float64 `json:"z"`
	Timestamp int64   `json:"timestamp"`
}

// Provider is a native sensors backend. On mobile the shell registers one
// via SetProvider (SensorManager on Android, CoreMotion on iOS) and delivers
// readings through the bridge event channel. Desktop machines rarely have
// motion sensors; the JS bridge falls back to devicemotion/deviceorientation
// events where the WebView supports them.
type Provider interface {
	StartSensor(sensorType string) error
	StopSensor(sensorType string) error
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

func StartSensor(sensorType string) error {
	if p := getProvider(); p != nil {
		return p.StartSensor(sensorType)
	}
	return platformStartSensor(sensorType)
}

func StopSensor(sensorType string) error {
	if p := getProvider(); p != nil {
		return p.StopSensor(sensorType)
	}
	return platformStopSensor(sensorType)
}
