//go:build darwin && !mobilebuild && !js

package runtime

// macOS supports native windowing (in-process multi-window via glaze) and the
// system tray — the tray is implemented directly on purego/objc (NSStatusItem)
// so it shares glaze's fakecgo instead of pulling in gogpu/systray's colliding
// one. See tray_darwin.go / SPIKES.md.
const (
	platformWindowing = true
	platformTray      = true
)
