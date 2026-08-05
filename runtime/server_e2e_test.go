package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// End-to-end against a real listening server, rather than calling handlers
// directly. The token and origin checks are the production auth surface, so they
// are worth verifying through an actual HTTP round-trip: a middleware ordering
// mistake or a route registered without the checks would pass the handler-level
// tests and still ship an open bridge.
func TestProductionServerEnforcesTokenAndOrigin(t *testing.T) {
	bridge := NewBridge()
	bridge.Handle("test:echo", func(ctx context.Context, args json.RawMessage) (any, error) {
		return "echoed", nil
	})

	// Production mode (DevMode false) → a token is generated.
	srv, err := NewServer(Config{Title: "e2e"}, bridge)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = srv.Stop(stopCtx)
	})

	if srv.token == "" {
		t.Fatal("production server must have generated a token")
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Wait for the listener to accept.
	client := &http.Client{Timeout: 3 * time.Second}
	var ready bool
	for i := 0; i < 50; i++ {
		if resp, err := client.Get(base + "/api/health"); err == nil {
			resp.Body.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("server never became reachable")
	}

	post := func(token, origin string) int {
		req, err := http.NewRequest(http.MethodPost, base+"/api/invoke",
			strings.NewReader(`{"id":"1","method":"test:echo"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("X-Goleo-Token", token)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	// The happy path: correct token, no Origin (a native WebView client).
	if got := post(srv.token, ""); got != http.StatusOK {
		t.Errorf("valid token = %d, want 200", got)
	}
	// Missing token must be rejected — this is the case that used to pass
	// whenever generateToken had failed and left the token empty.
	if got := post("", ""); got != http.StatusUnauthorized {
		t.Errorf("missing token = %d, want 401", got)
	}
	if got := post("wrong-token", ""); got != http.StatusUnauthorized {
		t.Errorf("wrong token = %d, want 401", got)
	}
	// Even WITH the right token, a foreign browser origin is refused in
	// production — the token can leak into a page (it is injected as a global),
	// so the origin check is the second layer.
	if got := post(srv.token, "https://evil.com"); got != http.StatusForbidden {
		t.Errorf("foreign origin = %d, want 403", got)
	}
	// The app's own loopback origin is fine.
	if got := post(srv.token, fmt.Sprintf("http://127.0.0.1:%d", port)); got != http.StatusOK {
		t.Errorf("own origin = %d, want 200", got)
	}
}
