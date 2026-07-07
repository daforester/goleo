package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
)

type Server struct {
	config   Config
	bridge   *Bridge
	server   *http.Server
	listener net.Listener
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

	if s.config.DevMode {
		mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok","mode":"dev"}`))
		})
	} else {
		mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok","mode":"production"}`))
		})

		if s.config.EmbedFS != nil {
			if efs, ok := s.config.EmbedFS.(fs.FS); ok {
				feFS, err := fs.Sub(efs, "frontend/dist")
				if err != nil {
					feFS = efs
				}
				fileServer := http.FileServer(http.FS(feFS))
				mux.Handle("/", fileServer)
			}
		}
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.config.Port))
	if err != nil {
		listener, err = net.Listen("tcp", ":0")
		if err != nil {
			return 0, fmt.Errorf("failed to listen: %w", err)
		}
	}

	s.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

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
	data, _ := json.Marshal(msg)
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

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
