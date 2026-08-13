package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/daforester/goleo/runtime/notify"
)

type InvokeHandler func(ctx context.Context, args json.RawMessage) (any, error)
type EventHandler func(ctx context.Context, data json.RawMessage)

type InvokeRequest struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args,omitempty"`
}

type InvokeResponse struct {
	ID     string `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

type EventMessage struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type Bridge struct {
	handlers    map[string]InvokeHandler
	events      map[string][]EventHandler
	subscribers []chan EventMessage
	mu          sync.RWMutex
	pending     map[string]chan InvokeResponse
	policy      *Policy
	// scope confines the fs plugin. Lazily created so a Bridge built by
	// NewBridge or as a zero value both behave.
	scope     *fsScopeState
	scopeOnce sync.Once
}

// fsScope returns this Bridge's filesystem confinement state, creating it on
// first use.
func (b *Bridge) fsScope() *fsScopeState {
	b.scopeOnce.Do(func() {
		if b.scope == nil {
			b.scope = newFSScopeState()
		}
	})
	return b.scope
}

// SetPolicy installs a capability ACL enforced on every invoke. Passing nil
// disables enforcement (the default). See Policy.
//
// A policy's FSRoots are also registered as filesystem roots here — that is what
// makes them actually confine the fs plugin. Before this, FSRoots was inert.
func (b *Bridge) SetPolicy(p *Policy) {
	b.mu.Lock()
	b.policy = p
	b.mu.Unlock()
	if p != nil {
		for _, root := range p.FSRoots {
			b.AddFSRoot(root)
		}
	}
}

func NewBridge() *Bridge {
	return &Bridge{
		handlers:    make(map[string]InvokeHandler),
		events:      make(map[string][]EventHandler),
		subscribers: make([]chan EventMessage, 0),
		pending:     make(map[string]chan InvokeResponse),
		scope:       newFSScopeState(),
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
	return b.HandleRequestContext(context.Background(), req)
}

// HandleRequestContext is HandleRequest with a caller-supplied context, passed through to
// the handler.
//
// Added for the JS runtime, and the reason is worth stating because it is not obvious: the
// embedded engine owns its VM on a single goroutine, so a script calling into Go runs the
// handler ON that goroutine. If the handler then calls back into JS, the goroutine would be
// waiting on itself — a deadlock. jsruntime_call.go marks its context so a nested call runs
// inline instead of queueing, and that marker can only reach the handler if the context
// does. HandleRequest kept context.Background() and therefore could not carry it.
//
// Every transport that does NOT have that constraint (HTTP, WebSocket, native IPC) should
// keep calling HandleRequest.
func (b *Bridge) HandleRequestContext(ctx context.Context, req InvokeRequest) InvokeResponse {
	b.mu.RLock()
	pol := b.policy
	fn, ok := b.handlers[req.Method]
	b.mu.RUnlock()

	// Central capability enforcement: a set policy denies any method not
	// explicitly allowed (deny-by-default), before the handler runs.
	if pol != nil && !pol.allowsMethod(req.Method) {
		return InvokeResponse{
			ID:    req.ID,
			Error: fmt.Sprintf("permission denied: %s", req.Method),
		}
	}

	if !ok {
		return InvokeResponse{
			ID:    req.ID,
			Error: fmt.Sprintf("method not found: %s", req.Method),
		}
	}

	result, err := fn(ctx, req.Args)
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

	// goleo:capabilities lets the frontend check which desktop features the
	// running platform supports before calling them. Registered for every
	// target (desktop, mobile, PWA) so the check is always answerable.
	b.Handle("goleo:capabilities", func(ctx context.Context, args json.RawMessage) (any, error) {
		return map[string]bool{
			"windowing": WindowingSupported(),
			"tray":      TraySupported(),
			"menu":      MenuSupported(),
		}, nil
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
		err := Notify(req.Title, req.Body)
		if errors.Is(err, notify.ErrPermissionNotGranted) {
			// Ask once, then retry. Android 13+ requires POST_NOTIFICATIONS to
			// be granted at runtime, and a first notify that does nothing
			// because nobody ever prompted is the common failure. The request
			// is asynchronous there — it puts the system dialog on the UI
			// thread and returns "default" — so this call cannot also deliver.
			// Report that state, so a caller can tell "ask the user and retry"
			// apart from "denied, stop asking".
			switch state := RequestNotificationPermission(); state {
			case "granted":
				err = Notify(req.Title, req.Body)
			case "denied":
				return nil, fmt.Errorf("%w: enable notifications for this app in system settings", notify.ErrPermissionNotGranted)
			default:
				return nil, fmt.Errorf("%w: requested (%s) — allow notifications, then retry", notify.ErrPermissionNotGranted, state)
			}
		}
		if err != nil {
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
