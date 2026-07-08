//go:build (android || ios) && goleo_geolocation

package geolocation

import "errors"

// On mobile the location service is only reachable from the native shell,
// which must register a Provider via SetProvider at startup. Without one the
// JS bridge falls back to navigator.geolocation in the WebView.

func platformGetCurrentPosition(opts PositionOptions) (*Position, error) {
	return nil, errors.New("geolocation: no native provider registered: the mobile shell must call SetProvider at startup")
}
