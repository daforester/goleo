package runtime

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// /api/invoke used to check only the token, never the Origin. In dev (no token)
// a cross-site fetch with Content-Type: text/plain is a CORS-*simple* request —
// no preflight — so it was sent and executed; the attacker cannot read the reply
// but the side effect (write a file, delete a directory) still lands.
func TestHandleInvokeRejectsCrossOrigin(t *testing.T) {
	cfg := Config{DevMode: true}
	s := &Server{
		config:         cfg,
		bridge:         NewBridge(),
		allowedOrigins: defaultAllowedOrigins(9842, cfg),
	}

	newReq := func(origin string) *httptest.ResponseRecorder {
		body := strings.NewReader(`{"id":"1","method":"goleo:getArch"}`)
		r := httptest.NewRequest(http.MethodPost, "/api/invoke", body)
		r.Header.Set("Content-Type", "text/plain")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		s.handleInvoke(w, r)
		return w
	}

	if got := newReq("https://evil.com").Code; got != http.StatusForbidden {
		t.Errorf("cross-origin invoke = %d, want %d", got, http.StatusForbidden)
	}
	// A legitimate dev origin still works.
	if got := newReq("http://localhost:5173").Code; got == http.StatusForbidden {
		t.Error("a loopback dev origin must not be rejected")
	}
	// And a native client with no Origin header.
	if got := newReq("").Code; got == http.StatusForbidden {
		t.Error("an empty Origin (native WebView) must not be rejected")
	}
}

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

// Dev is permissive about *where* the frontend is served from, but must not be a
// blanket yes — it used to return true unconditionally, so any page the user
// visited could open ws://127.0.0.1:<port>/ws and drive every registered bridge
// method (including the filesystem plugin) and read the replies.
func TestOriginOK_DevRejectsPublicOrigins(t *testing.T) {
	cfg := Config{DevMode: true}
	dev := &Server{config: cfg, allowedOrigins: defaultAllowedOrigins(9842, cfg)}

	// Legitimate dev/device-testing origins: loopback and private ranges, any port.
	for _, ok := range []string{
		"http://localhost:5173",
		"http://127.0.0.1:1234",
		"http://[::1]:5173",
		"http://10.0.2.2:5173",     // android emulator host alias
		"http://192.168.1.20:5173", // real-device LAN testing
		"http://172.16.4.4:8080",
		"", // native WebView client
	} {
		if !dev.originOK(ok) {
			t.Errorf("dev must allow %q", ok)
		}
	}

	// The actual attack: a page on the public web.
	for _, bad := range []string{
		"https://evil.com",
		"http://evil.com:5173",
		"https://goleo.dev",
		"http://8.8.8.8:5173",
	} {
		if dev.originOK(bad) {
			t.Errorf("dev must reject public origin %q", bad)
		}
	}
}

func TestDevExtraOriginsEscapeHatch(t *testing.T) {
	cfg := Config{DevMode: true}
	dev := &Server{config: cfg, allowedOrigins: defaultAllowedOrigins(9842, cfg)}
	if dev.originOK("https://tunnel.example.com") {
		t.Fatal("precondition: a public tunnel origin should be rejected by default")
	}
	t.Setenv("GOLEO_DEV_ALLOWED_ORIGINS", "https://tunnel.example.com, https://other.example")
	if !dev.originOK("https://tunnel.example.com") {
		t.Error("GOLEO_DEV_ALLOWED_ORIGINS must allow-list an explicit origin")
	}
	if !dev.originOK("https://other.example") {
		t.Error("comma-separated entries must each be honoured (whitespace trimmed)")
	}
	if dev.originOK("https://unlisted.example") {
		t.Error("an unlisted public origin must still be rejected")
	}
}

// generateToken must fail closed rather than returning "" — an empty token makes
// tokenOK accept anything, so a silent empty would remove production auth.
func TestGenerateTokenFailsClosed(t *testing.T) {
	tok, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	if len(tok) != 32 { // 16 random bytes, hex-encoded
		t.Errorf("token = %q (len %d), want 32 hex chars", tok, len(tok))
	}
	// A configured token must never be satisfied by an empty presented value.
	s := &Server{token: tok}
	if s.tokenOK("") {
		t.Error("a configured token must reject an empty presented token")
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
