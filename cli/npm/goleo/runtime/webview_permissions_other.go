//go:build !linux && !mobilebuild

package runtime

import "unsafe"

// enableWebviewPermissions is a no-op on non-Linux desktops. Windows (WebView2)
// and macOS (WKWebView) have their own permission models: WebView2 raises a
// PermissionRequested event, and WKWebView additionally requires camera/mic
// usage strings in the app's Info.plist. Neither is wired up here yet.
func enableWebviewPermissions(_ unsafe.Pointer) {}
