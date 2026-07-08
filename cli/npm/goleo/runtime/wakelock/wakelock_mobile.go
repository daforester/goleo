//go:build (android || ios) && goleo_wakelock

package wakelock

import "errors"

// On mobile the wake lock is only reachable from the native shell
// (FLAG_KEEP_SCREEN_ON on Android, isIdleTimerDisabled on iOS), which must
// register a Provider via SetProvider at startup. Without one the JS bridge
// falls back to navigator.wakeLock.

var errNoProvider = errors.New("wakelock: no native provider registered: the mobile shell must call SetProvider at startup")

func platformRequest(typeName string) error {
	return errNoProvider
}

func platformRelease() error {
	return errNoProvider
}
