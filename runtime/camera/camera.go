//go:build !(android || ios) || goleo_camera

package camera

import "sync"

type PhotoData struct {
	Data   []byte `json:"data"`
	Format string `json:"format"`
}

// Provider is a native camera backend. On mobile the shell registers one via
// SetProvider (CameraX on Android, AVFoundation on iOS). On desktop the
// webview's getUserMedia is the intended capture path, so the JS bridge
// falls back to it when no provider is set.
type Provider interface {
	CapturePhoto() (*PhotoData, error)
	StartStream(opts map[string]any) error
	StopStream() error
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

func CapturePhoto() (*PhotoData, error) {
	if p := getProvider(); p != nil {
		return p.CapturePhoto()
	}
	return platformCapturePhoto()
}

func StartStream(opts map[string]any) error {
	if p := getProvider(); p != nil {
		return p.StartStream(opts)
	}
	return platformStartStream(opts)
}

func StopStream() error {
	if p := getProvider(); p != nil {
		return p.StopStream()
	}
	return platformStopStream()
}
