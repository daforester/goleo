package runtime

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
)

type Server struct {
	config   Config
	bridge   *Bridge
	server   *http.Server
	listener net.Listener

	// token is a per-launch secret required on the bridge in production. It is
	// delivered to the frontend out of the file server (injected into
	// index.html as window.__GOLEO_TOKEN__) and echoed back on the WS handshake
	// (?token=) and /api/invoke (X-Goleo-Token). Empty in dev mode, where the
	// Vite dev server serves the HTML and localhost-only development is trusted.
	token string
	// allowedOrigins gates the WS upgrade and CORS: the app's own loopback
	// origins (and, in dev, the Vite origin). Blocks a malicious page in the
	// user's browser from driving the bridge over the loopback port.
	allowedOrigins []string
}

func NewServer(cfg Config, bridge *Bridge) (*Server, error) {
	return &Server{
		config: cfg,
		bridge: bridge,
	}, nil
}

func (s *Server) Start(ctx context.Context) (int, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/api/invoke", s.handleInvoke)

	mode := "production"
	if s.config.DevMode {
		mode = "dev"
	}
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","mode":%q}`, mode)
	})

	if !s.config.DevMode && s.config.EmbedFS != nil {
		if efs, ok := s.config.EmbedFS.(fs.FS); ok {
			feFS, err := fs.Sub(efs, "frontend/dist")
			if err != nil {
				feFS = efs
			}
			mux.Handle("/", s.staticHandler(feFS))
		}
	}

	// Bind loopback-only: the bridge must never be reachable from the network,
	// only from processes on this machine (and, in production, only with the
	// token). Falls back to an OS-assigned port if the configured one is taken.
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.config.Port))
	if err != nil {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, fmt.Errorf("failed to listen: %w", err)
		}
	}

	s.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	s.allowedOrigins = defaultAllowedOrigins(port, s.config)
	if !s.config.DevMode {
		// Fail CLOSED: if we cannot generate a token we must not start a server
		// whose only auth control silently accepts everything.
		token, terr := generateToken()
		if terr != nil {
			listener.Close()
			return 0, terr
		}
		s.token = token
	}

	s.server = &http.Server{
		Handler: s.corsMiddleware(mux),
	}

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()

	go s.handleEvents(ctx)

	return port, nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleEvents(ctx context.Context) {
	ch := s.bridge.Subscribe()
	defer s.bridge.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			s.broadcastEvent(msg)
		}
	}
}

func (s *Server) broadcastEvent(msg EventMessage) {
	conns := hub.GetAll()
	// Wrap in the same {type, data} envelope the client's handleMessage
	// switches on (matching the invokeResult/pong frames in websocket.go);
	// without the "event" type the frontend logs "unknown message type".
	data, _ := json.Marshal(map[string]any{
		"type": "event",
		"data": msg,
	})
	for _, conn := range conns {
		select {
		case conn.send <- data:
		default:
		}
	}
}

func (s *Server) handleInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check Origin here too, not just on the WS upgrade. A cross-site fetch with
	// Content-Type: text/plain is a CORS-*simple* request, so it is sent with no
	// preflight and the side effect lands even though the attacker cannot read
	// the response — blind CSRF against every registered method.
	if !s.originOK(r.Header.Get("Origin")) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}

	if !s.tokenOK(r.Header.Get("X-Goleo-Token")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var req InvokeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	resp := s.bridge.HandleRequest(req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.originOK(r.Header.Get("Origin")) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	if !s.tokenOK(r.URL.Query().Get("token")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	up := s.wsUpgrader()
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}

	c := &WSClient{
		conn: conn,
		send: make(chan []byte, 256),
	}
	hub.register <- c

	go c.writePump()
	go c.readPump(s.bridge)
}

// tokenOK reports whether the presented token is acceptable. When no token is
// configured (dev mode) the token is not the control — originOK is — so any
// presented value passes. When a token IS configured it must match exactly, and
// the comparison is constant-time so a caller can't recover it byte-by-byte from
// response timing.
func (s *Server) tokenOK(presented string) bool {
	if s.token == "" {
		return true
	}
	// Note the asymmetry with the old behaviour: an empty *presented* token no
	// longer passes just because it is empty — it must equal the configured one.
	return subtle.ConstantTimeCompare([]byte(presented), []byte(s.token)) == 1
}

// originOK reports whether a WS upgrade from origin is allowed. Enforced in
// production only: in dev/emulation the frontend is loaded cross-origin by
// design (e.g. `goleo emulate android` serves the UI from the host Vite server
// via http://10.0.2.2:<port> while the bridge connects to the in-app localhost
// backend), so enforcing the allow-list there would reject the legitimate
// upgrade and drop the app into local-only mode. Dev CORS is likewise permissive.
func (s *Server) originOK(origin string) bool {
	if originAllowed(origin, s.allowedOrigins) {
		return true
	}
	if s.config.DevMode && devOriginAllowed(origin) {
		return true
	}
	if s.config.DevMode {
		// Be loud. A silent 403 in dev reads as "goleo dev is broken", so name the
		// origin and the escape hatch.
		log.Printf("goleo: refused bridge connection from origin %q. In dev, loopback and "+
			"private-network origins are allowed; add others with GOLEO_DEV_ALLOWED_ORIGINS "+
			"(comma-separated).", origin)
	}
	return false
}

// devOriginAllowed relaxes the origin check in dev WITHOUT accepting the whole
// internet. Previously dev returned true unconditionally, which meant any page
// the user visited could open ws://127.0.0.1:<port>/ws and drive every registered
// bridge method — including the filesystem plugin — and read the replies.
//
// Dev legitimately needs cross-origin access from a moving target: the Vite dev
// server on any port, `goleo emulate android` reaching the host as 10.0.2.2, and
// real-device testing against a LAN address. All of those are loopback or private
// ranges, so allow those host classes (any port) plus an explicit env list, and
// reject public origins like https://evil.com.
func devOriginAllowed(origin string) bool {
	if origin == "" {
		return true // native/non-browser client
	}
	for _, extra := range devExtraOrigins() {
		if origin == extra {
			return true
		}
	}
	u, err := neturl.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false // a public hostname
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// devExtraOrigins reads GOLEO_DEV_ALLOWED_ORIGINS, the escape hatch for a dev
// setup served from an origin that is neither loopback nor private.
func devExtraOrigins() []string {
	raw := os.Getenv("GOLEO_DEV_ALLOWED_ORIGINS")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Reflect only origins that pass the same check the bridge uses: the app's
		// own allow-list, plus loopback/private-network origins in dev. Dev used to
		// reflect ANY origin, which handed credentialed cross-origin reads to any
		// site the user happened to be visiting.
		if origin != "" && s.originOK(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Goleo-Token")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// staticHandler serves the embedded frontend, injecting the bridge token into
// the root document so the frontend can authenticate without an extra request.
func (s *Server) staticHandler(feFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(feFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			if data, err := fs.ReadFile(feFS, "index.html"); err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(injectToken(data, s.token))
				return
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}

// --- hardening helpers (unit-tested in server_test.go) ---

// generateToken returns a per-launch bridge token. It returns an error rather
// than an empty string on CSPRNG failure: an empty token means tokenOK accepts
// anything, so failing open here would silently remove the production auth
// control instead of refusing to start.
func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("goleo: could not generate a bridge token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func defaultAllowedOrigins(port int, cfg Config) []string {
	origins := []string{
		fmt.Sprintf("http://127.0.0.1:%d", port),
		fmt.Sprintf("http://localhost:%d", port),
	}
	if cfg.DevMode {
		// Same resolution the window uses, so a project that moved its dev
		// server off 5173 is allow-listed as an origin too rather than having
		// its WS upgrade rejected.
		origins = append(origins, resolveDevServerURL(cfg))
	}
	return origins
}

// originAllowed permits an empty Origin (native/non-browser clients such as the
// desktop or mobile WebView, and CLI tools) and exact matches against the
// allow-list. A non-empty, non-matching Origin (e.g. a page in the user's
// browser) is rejected.
func originAllowed(origin string, allowed []string) bool {
	if origin == "" {
		return true
	}
	for _, a := range allowed {
		if origin == a {
			return true
		}
	}
	return false
}

// injectToken inserts window.__GOLEO_TOKEN__ into the document head so the
// bridge can read it. No-op when token is empty (dev mode).
func injectToken(html []byte, token string) []byte {
	if token == "" {
		return html
	}
	tag := "<script>window.__GOLEO_TOKEN__='" + token + "';</script>"
	doc := string(html)
	if i := strings.Index(doc, "</head>"); i >= 0 {
		return []byte(doc[:i] + tag + doc[i:])
	}
	return append([]byte(tag), html...)
}
