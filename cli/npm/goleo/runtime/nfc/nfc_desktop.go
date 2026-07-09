//go:build !android && !ios && !(linux && goleo_libnfc)

package nfc

import (
	"errors"
	"fmt"
)

// Desktop stub used unless the native libnfc backend is compiled in on Linux
// with -tags goleo_libnfc (see nfc_libnfc_linux.go).
var errUnsupported = fmt.Errorf("nfc: %w on desktop", errors.ErrUnsupported)

func platformStartScan() error {
	return errUnsupported
}

func platformStopScan() error {
	return errUnsupported
}

func platformWrite(message NFCMessage) error {
	return errUnsupported
}
