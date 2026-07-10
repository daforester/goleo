package runtime

import (
	"strings"
	"testing"
)

func TestInjectToken(t *testing.T) {
	html := []byte("<html><head><title>x</title></head><body></body></html>")

	out := string(injectToken(html, "abc123"))
	if !strings.Contains(out, "window.__GOLEO_TOKEN__='abc123'") {
		t.Fatalf("token not injected: %s", out)
	}
	if strings.Index(out, "__GOLEO_TOKEN__") >= strings.Index(out, "</head>") {
		t.Fatalf("token script should be inside <head>: %s", out)
	}

	// Empty token is a no-op (dev mode).
	if got := string(injectToken(html, "")); got != string(html) {
		t.Fatalf("empty token should not modify html, got: %s", got)
	}

	// No </head>: falls back to prepending so the token is still present.
	bare := []byte("<body>hi</body>")
	if out := string(injectToken(bare, "t")); !strings.HasPrefix(out, "<script>window.__GOLEO_TOKEN__='t'") {
		t.Fatalf("expected token prepended, got: %s", out)
	}
}

func TestOriginAllowed(t *testing.T) {
	allowed := defaultAllowedOrigins(9842, Config{})
	cases := []struct {
		origin string
		want   bool
	}{
		{"", true},                        // native / non-browser client
		{"http://127.0.0.1:9842", true},   // app's own origin
		{"http://localhost:9842", true},   // app's own origin (alt host)
		{"http://localhost:5173", false},  // Vite dev origin not allowed in prod
		{"http://evil.example", false},    // arbitrary page
		{"https://localhost:9842", false}, // scheme mismatch
	}
	for _, c := range cases {
		if got := originAllowed(c.origin, allowed); got != c.want {
			t.Errorf("originAllowed(%q) = %v, want %v", c.origin, got, c.want)
		}
	}

	// Dev mode additionally allows the Vite origin.
	devAllowed := defaultAllowedOrigins(9842, Config{DevMode: true})
	if !originAllowed("http://localhost:5173", devAllowed) {
		t.Error("dev mode should allow the Vite origin")
	}
}

func TestOriginOK_DevIsPermissive(t *testing.T) {
	// Regression: `goleo emulate android` loads the UI from http://10.0.2.2:<port>
	// (host Vite) but connects the bridge to the in-app localhost backend. In dev
	// mode that cross-origin WS upgrade must be allowed, or the app drops into
	// local-only mode ("backend not available").
	dev := &Server{config: Config{DevMode: true}, allowedOrigins: defaultAllowedOrigins(9842, Config{DevMode: true})}
	if !dev.originOK("http://10.0.2.2:5173") {
		t.Error("dev mode must allow the emulator's cross-origin WS upgrade")
	}

	// Production enforces the allow-list.
	prod := &Server{config: Config{}, allowedOrigins: defaultAllowedOrigins(9842, Config{})}
	if prod.originOK("http://10.0.2.2:5173") {
		t.Error("production must reject a foreign origin")
	}
	if !prod.originOK("http://127.0.0.1:9842") {
		t.Error("production must allow its own origin")
	}
	if !prod.originOK("") {
		t.Error("production must allow empty origin (native WebView client)")
	}
}

func TestTokenOK(t *testing.T) {
	// No token configured (dev): everything passes.
	dev := &Server{}
	if !dev.tokenOK("") || !dev.tokenOK("anything") {
		t.Error("dev server (no token) should accept any token")
	}

	// Token configured (prod): only the exact token passes.
	prod := &Server{token: "secret"}
	if prod.tokenOK("") || prod.tokenOK("wrong") {
		t.Error("prod server should reject missing/wrong token")
	}
	if !prod.tokenOK("secret") {
		t.Error("prod server should accept the correct token")
	}
}
