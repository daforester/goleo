//go:build mobilebuild || js

package runtime

// Mobile (gomobile) and wasm/PWA builds have no desktop windowing or tray: the
// platform hosts a single WebView itself. Desktop-only entry points guard on
// these and return errors.ErrUnsupported instead of running.
const (
	platformWindowing = false
	platformTray      = false
)
