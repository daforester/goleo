//go:build !(android || ios) || goleo_nfc

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/nfc"
)

func RegisterNFC(b *Bridge) {
	// Let native NFC backends (e.g. the libnfc desktop scanner) push scanned
	// tags to the frontend as "nfc:tag" events.
	nfc.SetEventSink(func(event string, data any) {
		b.Emit(event, data)
	})

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

// NFCProvider, NFCMessage and NFCRecord are re-exported so shells (e.g. the
// gomobile bridge) can inject a native backend without importing the
// sub-package directly.
type NFCProvider = nfc.Provider
type NFCMessage = nfc.NFCMessage
type NFCRecord = nfc.NFCRecord

func SetNFCProvider(p NFCProvider) {
	nfc.SetProvider(p)
}

// EmitNFCTag lets a native NFC backend (e.g. the gomobile bridge) push a
// "nfc:tag" event without depending on the Provider interface for it.
func EmitNFCTag(uid string) {
	nfc.Emit("nfc:tag", map[string]any{"uid": uid})
}
