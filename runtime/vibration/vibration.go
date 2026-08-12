//go:build !(android || ios) || goleo_vibration

package vibration

import "sync"

// Provider is a native vibration backend. On mobile the shell registers one
// via SetProvider (Vibrator/VibratorManager on Android, UIFeedbackGenerator
// on iOS). Desktop hardware has no vibrator; the JS bridge falls back to
// navigator.vibrate() where the WebView supports it.
type Provider interface {
	// Vibrate runs a web-Vibration-API pattern: alternating vibrate/pause
	// durations in milliseconds.
	Vibrate(pattern []int64) error
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

func Vibrate(pattern []int64) error {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p != nil {
		return p.Vibrate(pattern)
	}
	return platformVibrate(pattern)
}
