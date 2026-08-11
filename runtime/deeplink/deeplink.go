// Package deeplink registers a custom URL scheme (myapp://) so links launch or
// wake the app. Windows uses HKCU\Software\Classes; Linux a
// x-scheme-handler .desktop entry; macOS declares the scheme in the .app
// Info.plist at bundle time (so runtime Register is a no-op there). Handling:
// the OS launches the app with the URL as an argument — on first launch it's in
// os.Args, and on a running app it arrives via single-instance forwarding.
package deeplink

import "strings"

// Register associates scheme with exePath so the OS routes <scheme>:// URLs to
// the app. Best-effort; unsupported platforms return errors.ErrUnsupported.
func Register(scheme, appName, exePath string) error {
	return platformRegister(scheme, appName, exePath)
}

// SchemeURL returns the first arg that is a <scheme>:// URL, or "".
func SchemeURL(scheme string, args []string) string {
	if scheme == "" {
		return ""
	}
	prefix := scheme + "://"
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			return a
		}
	}
	return ""
}

// desktopEntry is the Linux .desktop body that registers the scheme handler.
func desktopEntry(scheme, appName, exePath string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=" + appName + "\n" +
		"Exec=" + exePath + " %u\n" +
		"MimeType=x-scheme-handler/" + scheme + ";\n" +
		"NoDisplay=true\n"
}
