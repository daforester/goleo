package runtime

import (
	"errors"
	"fmt"
)

// WindowingSupported reports whether this platform/build can open additional
// native windows (see App.OpenWindow). False on mobile and wasm/PWA builds,
// where the platform hosts a single WebView itself. Developer code can check
// this before calling windowing APIs; the APIs also guard internally.
func WindowingSupported() bool { return platformWindowing }

// TraySupported reports whether this platform/build can show a system tray
// icon. False on mobile and wasm/PWA builds.
func TraySupported() bool { return platformTray }

// requireWindowing returns an errors.ErrUnsupported-wrapped error when native
// windowing is unavailable, so callers can react with errors.Is(err,
// errors.ErrUnsupported) — matching the host-feature convention.
func requireWindowing() error {
	if !platformWindowing {
		return fmt.Errorf("goleo: windowing is not supported on this platform: %w", errors.ErrUnsupported)
	}
	return nil
}

// requireTray guards the (forthcoming) system-tray API the same way. It is the
// wrapper the tray entry points will call once that feature lands.
func requireTray() error {
	if !platformTray {
		return fmt.Errorf("goleo: system tray is not supported on this platform: %w", errors.ErrUnsupported)
	}
	return nil
}

var _ = requireTray // referenced once the tray API is implemented
