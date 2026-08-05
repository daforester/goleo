//go:build (android || ios) && goleo_clipboard

package clipboard

import "errors"

// On mobile the clipboard is only reachable from the native shell, which must
// register a Provider via SetProvider at startup. Without one the JS bridge
// falls back to navigator.clipboard.

func platformReadText() (string, error) {
	return "", errors.New("clipboard: no native provider registered: the mobile shell must call SetProvider at startup")
}

func platformWriteText(string) error {
	return errors.New("clipboard: no native provider registered: the mobile shell must call SetProvider at startup")
}
