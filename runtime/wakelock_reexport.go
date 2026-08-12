//go:build !(android || ios) || goleo_wakelock

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/wakelock"
)

func RegisterWakeLock(b *Bridge) {
	b.Handle("goleo:wakeLockRequest", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		return nil, wakelock.Request(req.Type)
	})
	b.Handle("goleo:wakeLockRelease", func(ctx context.Context, args json.RawMessage) (any, error) {
		return nil, wakelock.Release()
	})
}

// WakeLockProvider is re-exported so shells (e.g. the gomobile bridge) can
// inject a native backend without importing the sub-package directly.
type WakeLockProvider = wakelock.Provider

func SetWakeLockProvider(p WakeLockProvider) {
	wakelock.SetProvider(p)
}
