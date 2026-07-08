//go:build !(android || ios) || goleo_dialog

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/dialogs"
)

func RegisterDialogs(b *Bridge) {
	b.Handle("goleo:dialogOpenFile", func(ctx context.Context, args json.RawMessage) (any, error) {
		var opts dialogs.FileDialogOptions
		if len(args) > 0 {
			if err := json.Unmarshal(args, &opts); err != nil {
				return nil, err
			}
		}
		return dialogs.OpenFile(opts)
	})

	b.Handle("goleo:dialogSaveFile", func(ctx context.Context, args json.RawMessage) (any, error) {
		var opts dialogs.FileDialogOptions
		if len(args) > 0 {
			if err := json.Unmarshal(args, &opts); err != nil {
				return nil, err
			}
		}
		return dialogs.SaveFile(opts)
	})

	b.Handle("goleo:dialogSelectFolder", func(ctx context.Context, args json.RawMessage) (any, error) {
		var opts dialogs.FileDialogOptions
		if len(args) > 0 {
			if err := json.Unmarshal(args, &opts); err != nil {
				return nil, err
			}
		}
		return dialogs.SelectFolder(opts)
	})

	b.Handle("goleo:dialogShowMessage", func(ctx context.Context, args json.RawMessage) (any, error) {
		var opts dialogs.MessageBoxOptions
		if len(args) > 0 {
			if err := json.Unmarshal(args, &opts); err != nil {
				return nil, err
			}
		}
		btn, err := dialogs.ShowMessage(opts)
		if err != nil {
			return nil, err
		}
		return map[string]string{"button": btn}, nil
	})

	b.Handle("goleo:dialogShowPrompt", func(ctx context.Context, args json.RawMessage) (any, error) {
		var opts dialogs.PromptOptions
		if len(args) > 0 {
			if err := json.Unmarshal(args, &opts); err != nil {
				return nil, err
			}
		}
		return dialogs.ShowPrompt(opts)
	})
}

// DialogsProvider is re-exported so shells (e.g. the gomobile bridge) can
// inject a native backend without importing the sub-package directly.
type DialogsProvider = dialogs.Provider

func SetDialogsProvider(p DialogsProvider) {
	dialogs.SetProvider(p)
}
