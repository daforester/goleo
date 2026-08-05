//go:build (android || ios) && goleo_battery

package battery

import "errors"

// On mobile the battery service is only reachable from the native shell,
// which must register a Provider via SetProvider at startup. Without one the
// JS bridge falls back to navigator.getBattery().

func platformGetBatteryInfo() (*BatteryInfo, error) {
	return nil, errors.New("battery: no native provider registered: the mobile shell must call SetProvider at startup")
}
