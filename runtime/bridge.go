package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

type InvokeHandler func(ctx context.Context, args json.RawMessage) (any, error)
type EventHandler func(ctx context.Context, data json.RawMessage)

type InvokeRequest struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args,omitempty"`
}

type InvokeResponse struct {
	ID     string      `json:"id"`
	Result any         `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

type EventMessage struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type Bridge struct {
	handlers   map[string]InvokeHandler
	events     map[string][]EventHandler
	subscribers []chan EventMessage
	mu         sync.RWMutex
	pending    map[string]chan InvokeResponse
}

func NewBridge() *Bridge {
	return &Bridge{
		handlers:   make(map[string]InvokeHandler),
		events:     make(map[string][]EventHandler),
		subscribers: make([]chan EventMessage, 0),
		pending:    make(map[string]chan InvokeResponse),
	}
}

func (b *Bridge) Handle(name string, fn InvokeHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[name] = fn
}

func (b *Bridge) On(event string, fn EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events[event] = append(b.events[event], fn)
}

func (b *Bridge) Emit(event string, data any) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var raw json.RawMessage
	if data != nil {
		b, _ := json.Marshal(data)
		raw = b
	}

	msg := EventMessage{
		Event: event,
		Data:  raw,
	}

	for _, sub := range b.subscribers {
		select {
		case sub <- msg:
		default:
		}
	}
}

func (b *Bridge) HandleRequest(req InvokeRequest) InvokeResponse {
	b.mu.RLock()
	fn, ok := b.handlers[req.Method]
	b.mu.RUnlock()

	if !ok {
		return InvokeResponse{
			ID:    req.ID,
			Error: fmt.Sprintf("method not found: %s", req.Method),
		}
	}

	result, err := fn(context.Background(), req.Args)
	if err != nil {
		return InvokeResponse{
			ID:    req.ID,
			Error: err.Error(),
		}
	}

	return InvokeResponse{
		ID:     req.ID,
		Result: result,
	}
}

func (b *Bridge) DispatchEvent(event string, data json.RawMessage) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	handlers, ok := b.events[event]
	if !ok {
		return
	}
	for _, fn := range handlers {
		fn(context.Background(), data)
	}
}

func (b *Bridge) Subscribe() chan EventMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan EventMessage, 64)
	b.subscribers = append(b.subscribers, ch)
	return ch
}

func (b *Bridge) Unsubscribe(ch chan EventMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, sub := range b.subscribers {
		if sub == ch {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

func (b *Bridge) Call(method string, args any) (any, error) {
	var argsRaw json.RawMessage
	if args != nil {
		a, _ := json.Marshal(args)
		argsRaw = a
	}

	req := InvokeRequest{
		ID:     fmt.Sprintf("internal-%d", len(b.pending)+1),
		Method: method,
		Args:   argsRaw,
	}

	resp := b.HandleRequest(req)
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return resp.Result, nil
}

func RegisterBuiltins(b *Bridge) {
	registerCore(b)
}

// registerCore registers the always-safe built-in handlers that require no
// platform permissions and are available on every target (desktop, mobile, PWA).
func registerCore(b *Bridge) {
	b.Handle("goleo:getOS", func(ctx context.Context, args json.RawMessage) (any, error) {
		return GetOSInfo(), nil
	})

	b.Handle("goleo:getPlatform", func(ctx context.Context, args json.RawMessage) (any, error) {
		return GetPlatformInfo(), nil
	})

	b.Handle("goleo:getArch", func(ctx context.Context, args json.RawMessage) (any, error) {
		return GetArchInfo(), nil
	})

	b.Handle("goleo:getEnv", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		return GetEnvInfo(req.Key), nil
	})

	b.Handle("goleo:openURL", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		return nil, OpenURL(req.URL)
	})

	b.Handle("goleo:notify", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Title   string `json:"title"`
			Body    string `json:"body"`
			Message string `json:"message"` // legacy alias for body
		}
		if len(args) > 0 {
			if err := json.Unmarshal(args, &req); err != nil {
				return nil, fmt.Errorf("invalid args: %w", err)
			}
		}
		if req.Body == "" {
			req.Body = req.Message
		}
		if err := Notify(req.Title, req.Body); err != nil {
			return nil, err
		}
		return nil, nil
	})

	b.Handle("goleo:notificationPermissionGranted", func(ctx context.Context, args json.RawMessage) (any, error) {
		return NotificationPermissionGranted(), nil
	})

	b.Handle("goleo:requestNotificationPermission", func(ctx context.Context, args json.RawMessage) (any, error) {
		return RequestNotificationPermission(), nil
	})

	b.Handle("goleo:showMessage", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Title   string `json:"title"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		log.Printf("[goleo:showMessage] %s: %s", req.Title, req.Message)
		return nil, nil
	})
}
