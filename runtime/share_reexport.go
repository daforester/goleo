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
		// Same guard as goleo:openURL. On desktop, share hands data.URL to the OS
		// default handler (rundll32 url.dll / open / xdg-open), so without this a
		// file:// URL, a UNC path or a bare path to an executable is arbitrary
		// execution from any script in the webview. openURL was hardened for
		// exactly this and share was missed — it reaches the same handlers.
		if data.URL != "" {
			if err := checkOutboundURL("share", data.URL); err != nil {
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
