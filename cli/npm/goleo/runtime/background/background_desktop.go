//go:build !android && !ios

package background

import (
	"errors"
	"fmt"
)

// A desktop app process runs continuously, so OS-scheduled background sync
// does not apply; schedule work in-process instead.
var errUnsupported = fmt.Errorf("background: %w on desktop (the app process is always running)", errors.ErrUnsupported)

func platformRegisterSync(tag string) error {
	return errUnsupported
}

func platformGetPermission() bool {
	return false
}

func platformRequestPermission() error {
	return errUnsupported
}
