//go:build android || ios

package runtime

// RegisterDesktopFeatures is a no-op on mobile. It exists so shared app
// startup code (see the backend/app package in scaffolded projects) can call
// it unconditionally without per-target build tags — desktop-only extras
// (clipboard/dialogs/fs) are handled by desktop.go instead.
func RegisterDesktopFeatures(b *Bridge) {}
