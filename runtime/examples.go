package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

func RegisterSampleCommands(b *Bridge) {
	b.Handle("goleo:example:hello", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			req.Name = "World"
		}
		if req.Name == "" {
			req.Name = "World"
		}
		return map[string]string{
			"message": fmt.Sprintf("Hello, %s! From Goleo backend.", req.Name),
		}, nil
	})

	b.Handle("goleo:example:ping", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Timestamp int64 `json:"timestamp"`
		}
		json.Unmarshal(args, &req)
		return map[string]any{
			"pong":       true,
			"sent":       req.Timestamp,
			"received":   time.Now().UnixMilli(),
			"serverTime": time.Now().Format(time.RFC3339),
		}, nil
	})

	b.Handle("goleo:example:echo", func(ctx context.Context, args json.RawMessage) (any, error) {
		var data map[string]any
		if err := json.Unmarshal(args, &data); err != nil {
			return nil, fmt.Errorf("invalid data: %w", err)
		}
		return data, nil
	})

	log.Println("[goleo] sample commands registered (goleo:example:hello, goleo:example:ping, goleo:example:echo)")
}
