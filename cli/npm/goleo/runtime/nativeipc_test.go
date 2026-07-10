package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// testSession returns a native session whose pushes are captured on a channel
// (via the evalFn hook) instead of being evaluated in a real webview.
func testSession(a *App) (*nativeSession, <-chan string) {
	ch := make(chan string, 16)
	s := a.newNativeSession(nil)
	s.evalFn = func(jsonArg string) { ch <- jsonArg }
	return s, ch
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
	s, ch := testSession(a)

	s.onMessage(`{"type":"invoke","data":{"id":"7","method":"test:echo","args":{"x":1}}}`)

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
	s, ch := testSession(a)

	s.onMessage(`{"type":"invoke","data":{"id":"1","method":"test:secret"}}`)

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
	s, _ := testSession(a)

	s.onMessage(`{"type":"event","data":{"event":"app:ready","data":{"v":42}}}`)

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
	s, ch := testSession(a)

	s.onMessage(`{"type":"ping"}`)

	msg := awaitPush(t, ch)
	if msg["type"] != "pong" {
		t.Fatalf("type = %v, want pong", msg["type"])
	}
}

func TestNativeEventPumpForwardsEmits(t *testing.T) {
	a := New(Config{})
	a.ctx = context.Background()
	s, ch := testSession(a)

	stop := s.startEventPump()
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

func TestNativeEventPumpStopEndsPushes(t *testing.T) {
	a := New(Config{})
	a.ctx = context.Background()
	s, ch := testSession(a)

	stop := s.startEventPump()
	stop() // marks the session closed and unsubscribes

	a.bridge.Emit("data:updated", map[string]any{"count": 1})

	select {
	case s := <-ch:
		t.Fatalf("push after stop: %s", s)
	case <-time.After(200 * time.Millisecond):
		// expected: no further pushes once the pump is stopped
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
