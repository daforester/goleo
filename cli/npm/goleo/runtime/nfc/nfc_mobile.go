//go:build (android || ios) && goleo_nfc

package nfc

import "errors"

var errNoProvider = errors.New("nfc: no native provider registered: the mobile shell must call SetProvider at startup")

func platformStartScan() error {
	return errNoProvider
}

func platformStopScan() error {
	return errNoProvider
}

func platformWrite(message NFCMessage) error {
	return errNoProvider
}
