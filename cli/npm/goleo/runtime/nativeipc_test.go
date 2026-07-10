package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// collectPush wires a.nativeEvalFn to a channel so a test can await the JSON
// the backend would evaluate in the webview.
func collectPush(a *App) <-chan string {
	ch := make(chan string, 8)
	a.nativeEvalFn = func(jsonArg string) { ch <- jsonArg }
	return ch
}

func awaitPush(t *testing.T, ch <-chan string) map[string]any {
	t.Helper()
	select {
	case s := <-ch:
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("push not JSON: %v (%s)", err, s)
		}
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for native push")
		return nil
	}
}

func TestNativeInvokeRoundTrip(t *testing.T) {
	a := New(Config{})
	a.bridge.Handle("test:echo", func(ctx context.Context, args json.RawMessage) (any, error) {
		return map[string]string{"got": string(args)}, nil
	})
	ch := collectPush(a)

	a.onNativeMessage(`{"type":"invoke","data":{"id":"7","method":"test:echo","args":{"x":1}}}`)

	msg := awaitPush(t, ch)
	if msg["type"] != "invokeResult" {
		t.Fatalf("type = %v, want invokeResult", msg["type"])
	}
	data, _ := msg["data"].(map[string]any)
	if data["id"] != "7" {
		t.Errorf("id = %v, want 7", data["id"])
	}
	if data["error"] != nil {
		t.Errorf("unexpected error: %v", data["error"])
	}
	if data["result"] == nil {
		t.Error("missing result")
	}
}

func TestNativeInvokePolicyDenied(t *testing.T) {
	a := New(Config{})
	called := false
	a.bridge.Handle("test:secret", func(ctx context.Context, args json.RawMessage) (any, error) {
		called = true
		return "nope", nil
	})
	a.bridge.SetPolicy(&Policy{Allow: []string{"test:allowed"}})
	ch := collectPush(a)

	a.onNativeMessage(`{"type":"invoke","data":{"id":"1","method":"test:secret"}}`)

	msg := awaitPush(t, ch)
	data, _ := msg["data"].(map[string]any)
	if data["error"] == nil || data["error"] == "" {
		t.Error("policy should have denied the native invoke")
	}
	if called {
		t.Error("denied handler must not run")
	}
}

func TestNativeEventDispatch(t *testing.T) {
	a := New(Config{})
	got := make(chan string, 1)
	a.bridge.On("app:ready", func(ctx context.Context, data json.RawMessage) {
		got <- string(data)
	})

	a.onNativeMessage(`{"type":"event","data":{"event":"app:ready","data":{"v":42}}}`)

	select {
	case data := <-got:
		if data != `{"v":42}` {
			t.Errorf("event data = %s, want {\"v\":42}", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event was not dispatched to the bridge")
	}
}

func TestNativePing(t *testing.T) {
	a := New(Config{})
	ch := collectPush(a)

	a.onNativeMessage(`{"type":"ping"}`)

	msg := awaitPush(t, ch)
	if msg["type"] != "pong" {
		t.Fatalf("type = %v, want pong", msg["type"])
	}
}

func TestNativeEventPumpForwardsEmits(t *testing.T) {
	a := New(Config{})
	a.ctx = context.Background()
	ch := collectPush(a)

	stop := a.startNativeEventPump(nil) // nil window is fine: nativeEvalFn intercepts
	defer stop()

	a.bridge.Emit("data:updated", map[string]any{"count": 3})

	msg := awaitPush(t, ch)
	if msg["type"] != "event" {
		t.Fatalf("type = %v, want event", msg["type"])
	}
	data, _ := msg["data"].(map[string]any)
	if data["event"] != "data:updated" {
		t.Errorf("event = %v, want data:updated", data["event"])
	}
}

func TestNativeOnInitNilWhenDisabled(t *testing.T) {
	if New(Config{}).nativeOnInit() != nil {
		t.Error("nativeOnInit should be nil when NativeIPC is disabled")
	}
	if New(Config{NativeIPC: true}).nativeOnInit() == nil {
		t.Error("nativeOnInit should be non-nil when NativeIPC is enabled")
	}
}
