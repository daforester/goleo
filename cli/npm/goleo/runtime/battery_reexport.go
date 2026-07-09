//go:build !(android || ios) || goleo_battery

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/battery"
)

func RegisterBattery(b *Bridge) {
	b.Handle("goleo:batteryGetInfo", func(ctx context.Context, args json.RawMessage) (any, error) {
		return battery.GetBatteryInfo()
	})
}

// BatteryProvider and BatteryInfo are re-exported so shells (e.g. the
// gomobile bridge) can inject a native backend without importing the
// sub-package directly.
type BatteryProvider = battery.Provider
type BatteryInfo = battery.BatteryInfo

func SetBatteryProvider(p BatteryProvider) {
	battery.SetProvider(p)
}
