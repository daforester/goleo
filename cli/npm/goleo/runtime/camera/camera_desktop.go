//go:build !android && !ios && (!linux || !cgo)

package camera

import (
	"errors"
	"fmt"
)

// macOS/Windows desktop capture goes through the webview's getUserMedia (see
// bridge/src/camera.ts); native capture would need per-OS media stacks. Linux
// has a native V4L2 implementation in camera_linux.go, but only under cgo; a
// CGO_ENABLED=0 Linux build (the pure-Go webview path) falls back to this stub
// and the same getUserMedia route.
var errUnsupported = fmt.Errorf("camera: %w on desktop (use the getUserMedia fallback)", errors.ErrUnsupported)

func platformCapturePhoto() (*PhotoData, error) {
	return nil, errUnsupported
}

func platformStartStream(opts map[string]any) error {
	return errUnsupported
}

func platformStopStream() error {
	return errUnsupported
}
