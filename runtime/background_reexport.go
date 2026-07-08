//go:build !(android || ios) || goleo_background

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/background"
)

func RegisterBackground(b *Bridge) {
	b.Handle("goleo:backgroundRegisterSync", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		return nil, background.RegisterSync(req.Tag)
	})
	b.Handle("goleo:backgroundPermissionGranted", func(ctx context.Context, args json.RawMessage) (any, error) {
		return map[string]bool{"granted": background.GetPermission()}, nil
	})
	b.Handle("goleo:backgroundRequestPermission", func(ctx context.Context, args json.RawMessage) (any, error) {
		return nil, background.RequestPermission()
	})
}

// BackgroundProvider is re-exported so shells (e.g. the gomobile bridge) can
// inject a native backend without importing the sub-package directly.
type BackgroundProvider = background.Provider

func SetBackgroundProvider(p BackgroundProvider) {
	background.SetProvider(p)
}
