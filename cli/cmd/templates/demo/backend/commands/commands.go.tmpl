package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"time"

	goleoruntime "github.com/daforester/goleo/runtime"
)

func Register(b *goleoruntime.Bridge) {
	b.Handle("greet", func(ctx context.Context, args json.RawMessage) (any, error) {
		var params map[string]string
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, err
		}
		name := params["name"]
		if name == "" {
			name = "World"
		}
		return map[string]string{
			"message": fmt.Sprintf("Hello, %s! From Go backend at %s.", name, time.Now().Format(time.RFC3339)),
		}, nil
	})

	b.Handle("systemInfo", func(ctx context.Context, args json.RawMessage) (any, error) {
		return map[string]any{
			"goVersion":  runtime.Version(),
			"os":         runtime.GOOS,
			"arch":       runtime.GOARCH,
			"cpus":       runtime.NumCPU(),
			"goroutines": runtime.NumGoroutine(),
		}, nil
	})

	b.Handle("add", func(ctx context.Context, args json.RawMessage) (any, error) {
		var params map[string]float64
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("invalid args: need 'a' and 'b' numbers")
		}
		return map[string]float64{
			"result": params["a"] + params["b"],
		}, nil
	})

	b.Handle("countdown", func(ctx context.Context, args json.RawMessage) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
			return map[string]string{
				"message": "3 seconds have passed! Async works.",
			}, nil
		}
	})

	b.Handle("notify", func(ctx context.Context, args json.RawMessage) (any, error) {
		var params map[string]string
		if err := json.Unmarshal(args, &params); err != nil {
			params = map[string]string{"title": "Notification", "message": string(args)}
		}
		if params["title"] == "" {
			params["title"] = "Goleo"
		}
		if params["message"] == "" {
			params["message"] = "Hello from Go!"
		}
		if err := goleoruntime.Notify(params["title"], params["message"]); err != nil {
			return nil, err
		}
		b.Emit("notification:show", params)
		return map[string]string{"status": "sent"}, nil
	})

	b.On("app:log", func(ctx context.Context, data json.RawMessage) {
		var msg string
		if err := json.Unmarshal(data, &msg); err != nil {
			msg = string(data)
		}
		log.Printf("[app:log] %s", msg)
	})
}

// StartHeartbeat emits a "heartbeat" event every 5 seconds with the current
// server time and active goroutine count. Call this in OnStartup to stream
// live data to the frontend.
func StartHeartbeat(b *goleoruntime.Bridge) {
	go func() {
		for {
			time.Sleep(5 * time.Second)
			b.Emit("heartbeat", map[string]any{
				"time":       time.Now().Format(time.RFC3339),
				"goroutines": runtime.NumGoroutine(),
			})
		}
	}()
}
