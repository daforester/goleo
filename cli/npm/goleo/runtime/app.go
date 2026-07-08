package runtime

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type App struct {
	config  Config
	bridge  *Bridge
	server  *Server
	jsr     *JSRuntime
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
	// InitJS is the path to a JavaScript startup script that controls window
	// creation (createWindow/getConfig API). When set, the file must exist.
	// When empty, init.js then backend/init.js are tried; if neither exists
	// the window is created from this Config directly.
	InitJS      string
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

func (a *App) StartServer() (int, error) {
	ctx := context.Background()
	a.ctx = ctx

	if a.config.OnStartup != nil {
		a.config.OnStartup(ctx)
	}

	srv, err := NewServer(a.config, a.bridge)
	if err != nil {
		return 0, fmt.Errorf("failed to create server: %w", err)
	}
	a.server = srv

	port, err := srv.Start(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to start server: %w", err)
	}

	fmt.Printf("  Goleo app running on http://localhost:%d\n", port)
	return port, nil
}

func (a *App) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	a.ctx = ctx
	a.cancel = cancel

	port, err := a.StartServer()
	if err != nil {
		return err
	}

	if a.jsr == nil {
		a.jsr = NewJSRuntime(a.config, a)
	}
	a.jsr.port = port
	if err := a.jsr.Run(); err != nil {
		fmt.Printf("  Warning: init script: %v\n", err)
	}

	if a.config.WindowMode == WindowModeWebview {
		return a.runWebview(port)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-sig:
	}

	return a.shutdown()
}

func (a *App) runWebview(port int) error {
	var win *WebviewWindow

	if a.jsr != nil && a.jsr.win != nil {
		win = a.jsr.win
	} else {
		url := a.serverURL(port)
		w := NewWebviewWindow(windowConfig{
			Title:  a.config.Title,
			Width:  a.config.Width,
			Height: a.config.Height,
			Center: true,
			URL:    url,
		})
		win = &w
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sig:
		case <-a.ctx.Done():
		}
		win.Destroy()
	}()

	win.Run()

	return a.shutdown()
}

func (a *App) serverURL(port int) string {
	if a.config.DevMode {
		devServer := a.config.DevServer
		if devServer == "" {
			devServer = "http://localhost:5173"
		}
		return devServer
	}
	return "http://localhost:" + strconv.Itoa(port)
}

func (a *App) shutdown() error {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if a.config.OnShutdown != nil {
		a.config.OnShutdown(shutdownCtx)
	}

	a.bridge.Emit("app:shutdown", map[string]any{})
	a.jsr.Stop()
	a.server.Stop(shutdownCtx)

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
