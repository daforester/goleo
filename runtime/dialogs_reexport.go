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
		paths, err := dialogs.OpenFile(opts)
		if err != nil {
			return nil, err
		}
		// The user explicitly chose these, so grant the fs plugin access to them
		// even when they sit outside the configured roots. This is what keeps the
		// ordinary "pick a file, then read it" flow working with no configuration.
		for _, p := range paths {
			b.GrantFSPath(p)
		}
		return paths, nil
	})

	b.Handle("goleo:dialogSaveFile", func(ctx context.Context, args json.RawMessage) (any, error) {
		var opts dialogs.FileDialogOptions
		if len(args) > 0 {
			if err := json.Unmarshal(args, &opts); err != nil {
				return nil, err
			}
		}
		path, err := dialogs.SaveFile(opts)
		if err != nil {
			return nil, err
		}
		b.GrantFSPath(path) // user-chosen destination
		return path, nil
	})

	b.Handle("goleo:dialogSelectFolder", func(ctx context.Context, args json.RawMessage) (any, error) {
		var opts dialogs.FileDialogOptions
		if len(args) > 0 {
			if err := json.Unmarshal(args, &opts); err != nil {
				return nil, err
			}
		}
		dir, err := dialogs.SelectFolder(opts)
		if err != nil {
			return nil, err
		}
		// Grant the whole directory, so the app can work with files inside it.
		if dir != "" {
			b.AddFSRoot(dir)
		}
		return dir, nil
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
//
// The option types are re-exported for the same reason: Provider's methods take
// them, so a shell adapter cannot implement the interface without naming them.
// Without these aliases the generated gomobile package had to import
// runtime/dialogs directly, which is exactly what "inject without importing the
// sub-package" is meant to avoid.
type (
	DialogsProvider   = dialogs.Provider
	FileFilter        = dialogs.FileFilter
	FileDialogOptions = dialogs.FileDialogOptions
	MessageBoxOptions = dialogs.MessageBoxOptions
	PromptOptions     = dialogs.PromptOptions
)

func SetDialogsProvider(p DialogsProvider) {
	dialogs.SetProvider(p)
}
