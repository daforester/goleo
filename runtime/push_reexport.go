//go:build !(android || ios) || goleo_push

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/push"
)

func RegisterPush(b *Bridge) {
	b.Handle("goleo:pushSubscribe", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			ServerKey string `json:"serverKey"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		return push.Subscribe(req.ServerKey)
	})
	b.Handle("goleo:pushUnsubscribe", func(ctx context.Context, args json.RawMessage) (any, error) {
		return nil, push.Unsubscribe()
	})
	b.Handle("goleo:pushGetSubscription", func(ctx context.Context, args json.RawMessage) (any, error) {
		return push.GetSubscription()
	})
}

// PushProvider is re-exported so shells (e.g. the gomobile bridge) can
// inject a native backend without importing the sub-package directly.
type PushProvider = push.Provider

func SetPushProvider(p PushProvider) {
	push.SetProvider(p)
}
