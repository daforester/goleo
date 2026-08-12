// Package autostart registers/unregisters an app to launch on login. Desktop
// only: Windows uses the HKCU Run key, macOS a LaunchAgent plist, Linux an
// autostart .desktop file. Mobile/wasm report errors.ErrUnsupported.
package autostart

import (
	"strings"
)

// Enable registers the app (at exePath) to start on login under appName.
func Enable(appName, exePath string) error { return platformEnable(appName, exePath) }

// Disable removes the login entry for appName.
func Disable(appName string) error { return platformDisable(appName) }

// IsEnabled reports whether a login entry exists for appName.
func IsEnabled(appName string) (bool, error) { return platformIsEnabled(appName) }

// slug lowercases and hyphenates a name for filenames/identifiers.
func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteRune('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// desktopEntry is the Linux autostart .desktop file body.
func desktopEntry(appName, exePath string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=" + appName + "\n" +
		"Exec=" + exePath + "\n" +
		"X-GNOME-Autostart-enabled=true\n"
}

// launchAgentPlist is the macOS LaunchAgent plist body. label is the
// CFBundle-style identifier; exePath the program to run at load.
func launchAgentPlist(label, exePath string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>` + label + `</string>
	<key>ProgramArguments</key><array><string>` + exePath + `</string></array>
	<key>RunAtLoad</key><true/>
</dict>
</plist>
`
}
