//go:build !(android || ios) || goleo_share

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/share"
)

func RegisterShare(b *Bridge) {
	b.Handle("goleo:share", func(ctx context.Context, args json.RawMessage) (any, error) {
		var data share.ShareData
		if len(args) > 0 {
			if err := json.Unmarshal(args, &data); err != nil {
				return nil, err
			}
		}
		return nil, share.Share(&data)
	})
}

// ShareProvider and ShareData are re-exported so shells (e.g. the gomobile
// bridge) can inject a native backend without importing the sub-package.
type ShareProvider = share.Provider
type ShareData = share.ShareData

func SetShareProvider(p ShareProvider) {
	share.SetProvider(p)
}
