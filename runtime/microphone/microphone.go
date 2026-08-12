//go:build !(android || ios) || goleo_microphone

// Package microphone exposes the microphone's PERMISSION state, not audio capture.
//
// Recording is done in the WebView with getUserMedia + MediaRecorder, which works on
// every platform goleo targets and needs no Go code. What the WebView cannot do is tell
// you whether the OS has granted microphone access before you try, or ask for it without
// starting a capture — on mobile that is a native API. So this feature is deliberately
// small: two calls, and the Android manifest permission that registering it derives.
//
// It exists as its own feature rather than as part of Camera on purpose. RECORD_AUDIO is a
// permission users see and Play flags, and most camera apps only want stills; folding it
// into RegisterCamera would declare it for all of them. An app that wants camera + audio
// registers both.
package microphone

import "sync"

// Provider is a native microphone backend. On mobile the shell registers one via
// SetProvider (RECORD_AUDIO checks on Android, AVAudioSession on iOS). Desktop has no
// equivalent to query, so the JS bridge falls back to getUserMedia, whose own prompt is
// the permission model there.
//
// The method set mirrors notify.Notifier's deliberately: PermissionGranted/RequestPermission
// are shapes gobind binds cleanly to both Java and Swift (BOOL and NSString returns with no
// error result). A provider method returning (value, error) does NOT bind to Swift — see
// SPIKES.md, 2026-08-10.
type Provider interface {
	// PermissionGranted reports whether recording is currently allowed.
	PermissionGranted() bool
	// RequestPermission triggers the OS prompt and returns "granted", "denied" or
	// "default". "default" means the prompt is on screen and the answer is not known
	// yet — query again later rather than treating it as a refusal.
	RequestPermission() string
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

func PermissionGranted() (bool, error) {
	if p := getProvider(); p != nil {
		return p.PermissionGranted(), nil
	}
	return platformPermissionGranted()
}

func RequestPermission() (string, error) {
	if p := getProvider(); p != nil {
		return p.RequestPermission(), nil
	}
	return platformRequestPermission()
}
