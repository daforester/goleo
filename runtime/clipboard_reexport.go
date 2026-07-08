//go:build !(android || ios) || goleo_clipboard

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/clipboard"
)

func RegisterClipboard(b *Bridge) {
	b.Handle("goleo:clipboardReadText", func(ctx context.Context, args json.RawMessage) (any, error) {
		text, err := clipboard.ReadText()
		if err != nil {
			return nil, err
		}
		return map[string]string{"text": text}, nil
	})
	b.Handle("goleo:clipboardWriteText", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		return nil, clipboard.WriteText(req.Text)
	})
}

// ClipboardProvider is re-exported so shells (e.g. the gomobile bridge) can
// inject a native backend without importing the sub-package directly.
type ClipboardProvider = clipboard.Provider

func SetClipboardProvider(p ClipboardProvider) {
	clipboard.SetProvider(p)
}
