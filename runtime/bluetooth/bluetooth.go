//go:build !(android || ios) || goleo_ble

package bluetooth

import "sync"

type BLEDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	RSSI int    `json:"rssi"`
}

// Provider is a native BLE backend. On mobile the shell registers one via
// SetProvider (android.bluetooth.le / CoreBluetooth). On desktop there is no
// built-in stack; Chromium-based webviews expose Web Bluetooth as a
// fallback.
type Provider interface {
	RequestDevice(filters map[string]any) (*BLEDevice, error)
	Connect(deviceID string) error
	Disconnect(deviceID string) error
	Read(deviceID, service, characteristic string) ([]byte, error)
	Write(deviceID, service, characteristic string, data []byte) error
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

func RequestDevice(filters map[string]any) (*BLEDevice, error) {
	if p := getProvider(); p != nil {
		return p.RequestDevice(filters)
	}
	return platformRequestDevice(filters)
}

func Connect(deviceID string) error {
	if p := getProvider(); p != nil {
		return p.Connect(deviceID)
	}
	return platformConnect(deviceID)
}

func Disconnect(deviceID string) error {
	if p := getProvider(); p != nil {
		return p.Disconnect(deviceID)
	}
	return platformDisconnect(deviceID)
}

func Read(deviceID, service, characteristic string) ([]byte, error) {
	if p := getProvider(); p != nil {
		return p.Read(deviceID, service, characteristic)
	}
	return platformRead(deviceID, service, characteristic)
}

func Write(deviceID, service, characteristic string, data []byte) error {
	if p := getProvider(); p != nil {
		return p.Write(deviceID, service, characteristic, data)
	}
	return platformWrite(deviceID, service, characteristic, data)
}
