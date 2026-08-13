//go:build mobilebuild || js || (!windows && !linux && !darwin) || android || ios

// Geometry stub for every build with no native desktop window: mobile (the shell owns the
// window), PWA/wasm, and any OS without an implementation.
//
// Returns errors.ErrUnsupported rather than a generic error, because callers branch on it —
// window-state persistence checks for it to decide "this platform cannot restore geometry"
// versus "restoring failed", which are different situations and only one is worth logging.

package runtime

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"
)

func nativeGetRect(unsafe.Pointer) (WindowRect, error) {
	return WindowRect{}, fmt.Errorf("goleo: window geometry on %s: %w", runtime.GOOS, errors.ErrUnsupported)
}

func nativeSetRect(unsafe.Pointer, WindowRect) error {
	return fmt.Errorf("goleo: window geometry on %s: %w", runtime.GOOS, errors.ErrUnsupported)
}

func nativeSetChrome(unsafe.Pointer, WindowChrome) error {
	return fmt.Errorf("goleo: window chrome on %s: %w", runtime.GOOS, errors.ErrUnsupported)
}

func nativeVisibleBounds() (WindowRect, error) {
	return WindowRect{}, fmt.Errorf("goleo: visible bounds on %s: %w", runtime.GOOS, errors.ErrUnsupported)
}
