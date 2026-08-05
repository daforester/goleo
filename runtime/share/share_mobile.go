//go:build (android || ios) && goleo_share

package share

import "errors"

// On mobile the share sheet is only reachable from the native shell, which must
// register a Provider via SetProvider at startup (Android Intent.ACTION_SEND /
// iOS UIActivityViewController). Without one the JS bridge falls back to the
// Web Share API.
func platformShare(data *ShareData) error {
	return errors.New("share: no native provider registered: the mobile shell must call SetProvider at startup")
}
