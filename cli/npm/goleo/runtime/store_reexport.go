package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/daforester/goleo/runtime/store"
)

// RegisterStore exposes the persistent key/value store to the frontend. Unlike
// device features it needs no build tag or permission and works on every target
// (the Go backend owns a JSON file in the app data dir); the frontend falls
// back to localStorage when there is no backend (PWA).
func RegisterStore(b *Bridge) {
	b.Handle("goleo:storeGet", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		s, err := store.Default()
		if err != nil {
			return nil, err
		}
		val, found := s.Get(req.Key)
		return map[string]any{"value": val, "found": found}, nil
	})

	b.Handle("goleo:storeSet", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		s, err := store.Default()
		if err != nil {
			return nil, err
		}
		return nil, s.Set(req.Key, req.Value)
	})

	b.Handle("goleo:storeDelete", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		s, err := store.Default()
		if err != nil {
			return nil, err
		}
		return nil, s.Delete(req.Key)
	})

	b.Handle("goleo:storeKeys", func(ctx context.Context, args json.RawMessage) (any, error) {
		s, err := store.Default()
		if err != nil {
			return nil, err
		}
		return map[string][]string{"keys": s.Keys()}, nil
	})

	b.Handle("goleo:storeClear", func(ctx context.Context, args json.RawMessage) (any, error) {
		s, err := store.Default()
		if err != nil {
			return nil, err
		}
		return nil, s.Clear()
	})
}
