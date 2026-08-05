package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/daforester/goleo/runtime/updater"
)

// UpdaterConfig is re-exported so apps can configure the updater without
// importing the sub-package.
type UpdaterConfig = updater.Config

// RegisterUpdater exposes desktop auto-update to the frontend. Mobile/PWA apps
// update through their store, so this is opt-in and desktop-only. cfg carries
// the signed-manifest URL, the embedded ed25519 public key (base64), and the
// running app version.
func RegisterUpdater(b *Bridge, cfg UpdaterConfig) {
	client, initErr := updater.NewClient(cfg)

	b.Handle("goleo:updaterCheck", func(ctx context.Context, args json.RawMessage) (any, error) {
		if initErr != nil {
			return nil, initErr
		}
		rel, err := client.Check()
		if err != nil {
			return nil, err
		}
		if rel == nil {
			return map[string]any{"available": false}, nil
		}
		return map[string]any{"available": true, "version": rel.Version, "notes": rel.Notes}, nil
	})

	b.Handle("goleo:updaterApply", func(ctx context.Context, args json.RawMessage) (any, error) {
		if initErr != nil {
			return nil, initErr
		}
		rel, err := client.Check()
		if err != nil {
			return nil, err
		}
		if rel == nil {
			return nil, fmt.Errorf("updater: no update available")
		}
		path, err := client.Download(rel, func(done, total int64) {
			b.Emit("updater:progress", map[string]int64{"done": done, "total": total})
		})
		if err != nil {
			return nil, err
		}
		// On success this replaces the binary and relaunches (the process exits),
		// so the handler only returns here on failure.
		return nil, updater.ApplyAndRelaunch(path)
	})
}
