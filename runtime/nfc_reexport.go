//go:build !(android || ios) || goleo_nfc

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/nfc"
)

func RegisterNFC(b *Bridge) {
	b.Handle("goleo:nfcStartScan", func(ctx context.Context, args json.RawMessage) (any, error) {
		return nil, nfc.StartScan()
	})
	b.Handle("goleo:nfcStopScan", func(ctx context.Context, args json.RawMessage) (any, error) {
		return nil, nfc.StopScan()
	})
	b.Handle("goleo:nfcWrite", func(ctx context.Context, args json.RawMessage) (any, error) {
		var message nfc.NFCMessage
		if err := json.Unmarshal(args, &message); err != nil {
			return nil, err
		}
		return nil, nfc.Write(message)
	})
}

// NFCProvider is re-exported so shells (e.g. the gomobile bridge) can
// inject a native backend without importing the sub-package directly.
type NFCProvider = nfc.Provider

func SetNFCProvider(p NFCProvider) {
	nfc.SetProvider(p)
}
