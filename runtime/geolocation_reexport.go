//go:build !(android || ios) || goleo_geolocation

package runtime

// Geolocation is a PURE WEB feature: every platform reaches the OS through the
// WebView's navigator.geolocation, and there is no Go-side implementation at all.
//
// It did not start that way, and the Go side was never the real path:
//
//	Windows   WinRT Geolocator driven through a PowerShell 5.1 SUBPROCESS per call
//	          (the WinRT projection does not exist in pwsh 7+)
//	macOS     shelled out to CoreLocationCLI if `brew install corelocationcli`
//	          happened to be present, else ErrUnsupported
//	Linux     ErrUnsupported (would have needed a GeoClue D-Bus client)
//	Android    no provider — already navigator.geolocation
//	iOS        no provider — already navigator.geolocation
//
// So one of six platforms had a native path, and it was a process launch to fetch
// a coordinate. The other five already fell through to the browser, and Linux's
// WebKitGTK permission auto-grant exists in as many words "so the app's
// getUserMedia/geolocation fallbacks resolve instead of hanging" — the browser
// path was the design in practice long before it was the design on purpose.
//
// The webview is a BETTER caller of the same OS API: WebKit and WebView2 reach
// CoreLocation and the WinRT Geolocator themselves, with the real permission UI
// and no subprocess. Deleting the Go side removes the cgo-free workarounds
// (CoreLocation needs an Objective-C delegate, which pure Go cannot provide)
// rather than maintaining them.
//
// Camera and microphone are already web-only on every platform for the same
// reason, so this makes the feature set more consistent, not less.
//
// LIMIT, so nobody is surprised: navigator.geolocation only works while a page is
// alive and foregrounded. Background location — significant-change monitoring,
// geofencing — is not reachable this way and would need a real native provider in
// the mobile shells. That is the one case that would justify bringing one back.

// RegisterGeolocation declares that this app uses geolocation.
//
// It installs NO bridge command, and that is deliberate — the frontend calls
// navigator.geolocation directly through @goleo/bridge's getCurrentPosition().
// The call still has to exist, because it is what the CLI's manifest scanner
// detects, and that detection is what declares Android's ACCESS_FINE_LOCATION
// (plus the android.hardware.location* uses-feature entries) and iOS's
// NSLocationWhenInUseUsageDescription.
//
// Those declarations are not optional extras for the web path — they are what
// makes it work. Android's WebView can only grant a navigator.geolocation request
// if the app itself holds ACCESS_FINE_LOCATION, and WKWebView needs the usage
// description. So an app that stops calling this loses geolocation entirely,
// which is exactly the outcome a "this function is empty, delete it" cleanup
// would produce. TestGeolocationStaysDetectableAsAPureWebFeature guards it.
func RegisterGeolocation(b *Bridge) {
	// No handler by design. See above: this call is a build-time declaration that
	// the manifest scanner reads, not a runtime registration.
	_ = b
}
