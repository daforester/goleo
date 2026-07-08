//go:build !linux

package cmd

// prepareWebkitEnv is a no-op on non-Linux platforms. macOS uses the system
// WKWebView and Windows uses WebView2, so there is no pkg-config/WebKitGTK
// dependency to resolve.
func prepareWebkitEnv() ([]string, error) {
	return nil, nil
}
