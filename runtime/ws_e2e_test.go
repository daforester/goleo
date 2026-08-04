package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// End-to-end over a REAL WebSocket against a REAL server.
//
// Two gaps this closes. The hub tests register hand-built WSClient structs, so the
// hub had never been exercised by an actual connection going through the upgrade,
// readPump and writePump. And while nativeipc_test.go pins the {type,data} envelope
// for the native channel, nothing pinned it for the WebSocket transport — which is
// what mobile, the browser and every child-process window actually use.
//
// bridge/src/bridge.test.ts asserts the TypeScript side emits exactly this shape;
// this is the other half of that contract. If they drift, both suites keep passing
// while every real call fails.
func dialServer(t *testing.T, srv *Server, port int) *websocket.Conn {
	t.Helper()
	url := fmt.Sprintf("ws://127.0.0.1:%d/ws?token=%s", port, srv.token)
	var conn *websocket.Conn
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, resp, err := websocket.DefaultDialer.Dial(url, nil)
		if err == nil {
			conn = c
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(10 * time.Millisecond)
	}
	if conn == nil {
		t.Fatal("could not connect to the server")
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func startServer(t *testing.T, b *Bridge) (*Server, int) {
	t.Helper()
	srv, err := NewServer(Config{Title: "wse2e"}, b)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	port, err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = srv.Stop(stopCtx)
	})
	return srv, port
}

func TestWebSocketInvokeRoundTrip(t *testing.T) {
	b := NewBridge()
	b.Handle("test:add", func(ctx context.Context, args json.RawMessage) (any, error) {
		var p struct{ A, B int }
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return map[string]int{"sum": p.A + p.B}, nil
	})
	b.Handle("test:fail", func(ctx context.Context, args json.RawMessage) (any, error) {
		return nil, fmt.Errorf("fs: %q is outside the allowed roots", "/etc/x")
	})

	srv, port := startServer(t, b)
	conn := dialServer(t, srv, port)

	// Exactly the frame bridge.ts emits (see its "invoke wire format" tests).
	send := func(id, method string, args any) {
		t.Helper()
		raw, _ := json.Marshal(args)
		frame := map[string]any{
			"type": "invoke",
			"data": map[string]any{"id": id, "method": method, "args": json.RawMessage(raw)},
		}
		if err := conn.WriteJSON(frame); err != nil {
			t.Fatal(err)
		}
	}
	read := func() map[string]any {
		t.Helper()
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("read: %v", err)
		}
		return msg
	}

	send("1", "test:add", map[string]int{"A": 2, "B": 3})
	msg := read()
	if msg["type"] != "invokeResult" {
		t.Fatalf("type = %v, want invokeResult", msg["type"])
	}
	data := msg["data"].(map[string]any)
	if data["id"] != "1" {
		t.Errorf("id = %v, want 1", data["id"])
	}
	if sum := data["result"].(map[string]any)["sum"]; sum != float64(5) {
		t.Errorf("sum = %v, want 5", sum)
	}

	// A handler error must arrive as its own text: the fs and dialog wrappers now
	// rethrow it verbatim, so anything that flattens it breaks their diagnostics.
	send("2", "test:fail", nil)
	msg = read()
	data = msg["data"].(map[string]any)
	errText, _ := data["error"].(string)
	if errText == "" {
		t.Fatalf("expected an error field, got %v", data)
	}
	if want := "outside the allowed roots"; !strings.Contains(errText, want) {
		t.Errorf("error = %q, should contain %q", errText, want)
	}
}

// The hub tests use hand-built clients. This proves isolation holds for real
// connections too — the bug was that one process's Apps shared a global hub.
func TestEventsReachOnlyTheirOwnServersClients(t *testing.T) {
	bridgeA, bridgeB := NewBridge(), NewBridge()
	srvA, portA := startServer(t, bridgeA)
	srvB, portB := startServer(t, bridgeB)

	connA := dialServer(t, srvA, portA)
	connB := dialServer(t, srvB, portB)

	// Give both registrations time to land in their hubs.
	time.Sleep(150 * time.Millisecond)

	srvA.broadcastEvent(EventMessage{Event: "a:only"})

	connA.SetReadDeadline(time.Now().Add(3 * time.Second))
	var got map[string]any
	if err := connA.ReadJSON(&got); err != nil {
		t.Fatalf("client A never received its own server's event: %v", err)
	}
	if got["type"] != "event" {
		t.Errorf("type = %v, want event", got["type"])
	}

	// B must hear nothing at all.
	connB.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	var leaked map[string]any
	if err := connB.ReadJSON(&leaked); err == nil {
		t.Errorf("client B received server A's event: %v — hubs are shared", leaked)
	}
}

// A production server must refuse a socket with no token, over the real upgrade
// path rather than by calling tokenOK directly.
func TestWebSocketUpgradeRequiresTheToken(t *testing.T) {
	srv, port := startServer(t, NewBridge())
	if srv.token == "" {
		t.Fatal("production server should have a token")
	}

	_, resp, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err == nil {
		t.Fatal("upgrade without a token should have been refused")
	}
	if resp != nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	}
}
