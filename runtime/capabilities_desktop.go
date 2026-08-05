//go:build !mobilebuild && !js && !darwin

package runtime

// Windows + Linux desktop (macOS has its own capabilities_darwin.go): native
// windows, system tray, and a native menu bar (Win32 / GTK3). The mobilebuild /
// js counterpart reports these as unavailable so shared app code degrades.
const (
	platformWindowing = true
	platformTray      = true
	platformMenu      = true
)
