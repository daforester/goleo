package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
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
		s.token = generateToken()
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

	conn, err := upgrader.Upgrade(w, r, nil)
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
// configured (dev mode), every request passes.
func (s *Server) tokenOK(presented string) bool {
	return s.token == "" || presented == s.token
}

// originOK reports whether a WS upgrade from origin is allowed. Enforced in
// production only: in dev/emulation the frontend is loaded cross-origin by
// design (e.g. `goleo emulate android` serves the UI from the host Vite server
// via http://10.0.2.2:<port> while the bridge connects to the in-app localhost
// backend), so enforcing the allow-list there would reject the legitimate
// upgrade and drop the app into local-only mode. Dev CORS is likewise permissive.
func (s *Server) originOK(origin string) bool {
	return s.config.DevMode || originAllowed(origin, s.allowedOrigins)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Dev mode reflects any origin (local development convenience). In
		// production only the app's own allowed origins are permitted — no
		// wildcard reflection.
		if origin != "" && (s.config.DevMode || originAllowed(origin, s.allowedOrigins)) {
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

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fail open on the token (origin checks still apply) rather than crash.
		return ""
	}
	return hex.EncodeToString(b)
}

func defaultAllowedOrigins(port int, cfg Config) []string {
	origins := []string{
		fmt.Sprintf("http://127.0.0.1:%d", port),
		fmt.Sprintf("http://localhost:%d", port),
	}
	if cfg.DevMode {
		dev := cfg.DevServer
		if dev == "" {
			dev = "http://localhost:5173"
		}
		origins = append(origins, dev)
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
