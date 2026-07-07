package runtime

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type App struct {
	config  Config
	bridge  *Bridge
	server  *Server
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	ctx     context.Context
}

type Config struct {
	Title       string
	Width       int
	Height      int
	DevMode     bool
	DevServer   string
	Port        int
	WindowMode  WindowMode
	EmbedFS     any
	OnStartup   func(ctx context.Context)
	OnShutdown  func(ctx context.Context)
}

type WindowMode int

const (
	WindowModeBrowser WindowMode = iota
	WindowModeWebview
	WindowModeMobile
)

func New(cfg Config) *App {
	if cfg.Port == 0 {
		cfg.Port = 9842
	}
	if cfg.Title == "" {
		cfg.Title = "Goleo App"
	}
	if cfg.Width == 0 {
		cfg.Width = 1024
	}
	if cfg.Height == 0 {
		cfg.Height = 768
	}

	return &App{
		config: cfg,
		bridge: NewBridge(),
	}
}

func (a *App) Bridge() *Bridge {
	return a.bridge
}

func (a *App) Config() Config {
	return a.config
}

func (a *App) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	a.ctx = ctx
	a.cancel = cancel

	if a.config.OnStartup != nil {
		a.config.OnStartup(ctx)
	}

	srv, err := NewServer(a.config, a.bridge)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}
	a.server = srv

	port, err := srv.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	if a.config.OnStartup != nil {
		fmt.Printf("  Goleo app running on http://localhost:%d\n", port)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-sig:
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if a.config.OnShutdown != nil {
		a.config.OnShutdown(shutdownCtx)
	}

	a.bridge.Emit("app:shutdown", map[string]any{})
	srv.Stop(shutdownCtx)

	return nil
}

func (a *App) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) Invoke(name string, fn InvokeHandler) {
	a.bridge.Handle(name, fn)
}

func (a *App) On(event string, fn EventHandler) {
	a.bridge.On(event, fn)
}

func (a *App) Emit(event string, data any) {
	a.bridge.Emit(event, data)
}


