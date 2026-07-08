//go:build !(android || ios) || goleo_vibration

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/vibration"
)

func RegisterVibration(b *Bridge) {
	b.Handle("goleo:vibrate", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Pattern []int64 `json:"pattern"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		return nil, vibration.Vibrate(req.Pattern)
	})
}

// VibrationProvider is re-exported so shells (e.g. the gomobile bridge) can
// inject a native backend without importing the sub-package directly.
type VibrationProvider = vibration.Provider

func SetVibrationProvider(p VibrationProvider) {
	vibration.SetProvider(p)
}
