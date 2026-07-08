//go:build (android || ios) && goleo_vibration

package vibration

import "errors"

func platformVibrate(pattern []int64) error {
	return errors.New("vibration: no native provider registered: the mobile shell must call SetProvider at startup")
}
