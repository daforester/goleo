//go:build !android && !ios

package camera

import (
	"errors"
	"fmt"
)

// Desktop capture goes through the webview's getUserMedia (see
// bridge/src/camera.ts); native capture would need per-OS media stacks.
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
