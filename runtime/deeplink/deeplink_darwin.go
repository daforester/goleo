//go:build darwin

package deeplink

// On macOS a URL scheme is declared in the .app bundle's Info.plist
// (CFBundleURLTypes) at build time — see the bundler's url_scheme support — so
// there is no runtime registration step. Handling the incoming URL still
// requires the native app layer.
func platformRegister(scheme, appName, exePath string) error {
	return nil
}
