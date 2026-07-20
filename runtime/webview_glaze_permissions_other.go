//go:build (darwin || windows) && !mobilebuild

package runtime

import "unsafe"

// enableGlazePermissions is a no-op off Linux: macOS grants media/geolocation
// through the WKUIDelegate, WebView2 handles its own permission prompts, and the
// app loads only its own trusted content — so there is no permission-request
// signal to wire (only WebKitGTK needs one). See the Linux counterpart
// (webview_glaze_permissions_linux.go).
func enableGlazePermissions(window unsafe.Pointer) {}
