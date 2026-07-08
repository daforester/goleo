//go:build !android && !ios

package nfc

import (
	"errors"
	"fmt"
)

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
