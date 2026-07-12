//go:build !linux && !mobilebuild && goleo_glaze

package runtime

import "unsafe"

// enableGlazePermissions is a no-op off Linux: macOS grants media/geolocation
// through the WKUIDelegate (and the app loads only its own trusted content), and
// there is no WebKitGTK permission-request signal to wire. See the Linux
// counterpart (webview_glaze_permissions_linux.go).
func enableGlazePermissions(window unsafe.Pointer) {}
