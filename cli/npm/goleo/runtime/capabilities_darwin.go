//go:build darwin && !mobilebuild && !js

package runtime

// macOS supports native windowing (incl. in-process multi-window via glaze) but
// NOT the system tray with the cgo-free backend — glaze's and gogpu/systray's
// fakecgo shims collide at link time on Mach-O (see tray_darwin.go / SPIKES.md).
const (
	platformWindowing = true
	platformTray      = false
)
