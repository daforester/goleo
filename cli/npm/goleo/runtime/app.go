package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/daforester/goleo/runtime/autostart"
	"github.com/daforester/goleo/runtime/singleinstance"
)

type App struct {
	config  Config
	bridge  *Bridge
	server  *Server
	jsr      *JSRuntime
	windows  windowSpawner
	instance *singleinstance.Instance
	port     int
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	ctx     context.Context
}

type Config struct {
	Title      string
	Width      int
	Height     int
	DevMode    bool
	DevServer  string
	Port       int
	WindowMode WindowMode
	EmbedFS    any
	// InProcessWindows opts additional windows into the in-process model
	// (each on its own OS thread) instead of child processes. Windows only for
	// now; ignored elsewhere (falls back to multi-process). See spikes/win-multiwindow.
	InProcessWindows bool
	// SingleInstance, when true, allows only one running instance; a second
	// launch forwards its args to the running one (emitting app:secondInstance)
	// and exits. AppID identifies the app for the lock (defaults to Title).
	SingleInstance bool
	AppID          string
	// Background runs the app as a headless controller: no auto primary window
	// (open windows on demand via OpenWindow / the tray), and the main thread
	// runs the tray (if Tray is set) or blocks until Quit.
	Background bool
	// Tray adds a system tray icon + menu (used with Background). Desktop only.
	Tray *TrayConfig
	// OnReady runs (in a goroutine) once the server + window manager are up and
	// the port is known — where OpenWindow works. Unlike OnStartup, which runs
	// before the server binds.
	OnReady func(ctx context.Context)
	// InitJS is the path to a JavaScript startup script that controls window
	// creation (createWindow/getConfig API). When set, the file must exist.
	// When empty, init.js then backend/init.js are tried; if neither exists
	// the window is created from this Config directly.
	InitJS     string
	OnStartup  func(ctx context.Context)
	OnShutdown func(ctx context.Context)
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
	a.port = port

	fmt.Printf("  Goleo app running on http://localhost:%d\n", port)
	return port, nil
}

func (a *App) Run() error {
	// A child window process (spawned by WindowManager) just hosts one webview
	// pointed at the parent's server — no server, no init script, no lifecycle.
	if isWindowChild() {
		return a.runWindowChild()
	}

	// Single-instance: a second launch forwards its args to the running app
	// and exits, rather than starting a second backend. Done before the server
	// binds so a secondary never claims the port.
	if a.config.SingleInstance {
		appID := a.config.AppID
		if appID == "" {
			appID = a.config.Title
		}
		inst, primary, err := singleinstance.Acquire(appID, os.Args[1:], func(args []string) {
			a.Emit("app:secondInstance", map[string]any{"args": args})
		})
		if err != nil {
			fmt.Printf("  single-instance: %v — starting normally\n", err)
		} else if !primary {
			fmt.Println("  Another instance is already running — forwarded and exiting.")
			return nil
		} else {
			a.instance = inst
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.ctx = ctx
	a.cancel = cancel

	port, err := a.StartServer()
	if err != nil {
		return err
	}

	// Multi-window is a desktop-only capability driven from Run (mobile enters
	// through StartServer instead), so wire it up here once the port is known.
	// In-process windows are opt-in and (for now) Windows-only; everything else
	// uses the cross-platform multi-process manager.
	if a.config.InProcessWindows && runtime.GOOS == "windows" {
		a.windows = newInProcWindowManager(a)
	} else {
		a.windows = newWindowManager(a)
	}
	a.registerWindowCommands()

	if a.jsr == nil {
		a.jsr = NewJSRuntime(a.config, a)
	}
	a.jsr.port = port
	if err := a.jsr.Run(); err != nil {
		fmt.Printf("  Warning: init script: %v\n", err)
	}

	// Ready hook: server + window manager are up and the port is known, so
	// OpenWindow works here (run in a goroutine — the main thread is about to be
	// claimed by the webview/tray loop).
	if a.config.OnReady != nil {
		go a.config.OnReady(ctx)
	}

	// Background: headless controller — no in-process primary window. The main
	// thread runs the tray (if configured) or blocks until Quit.
	if a.config.Background {
		if a.config.Tray != nil {
			a.runTrayLoop() // blocks; Quit tears down + exits via the ctx watcher
			return nil
		}
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-ctx.Done():
		case <-sig:
		}
		return a.shutdown()
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

	if a.windows != nil {
		a.windows.CloseAll()
	}
	if a.instance != nil {
		a.instance.Close()
	}

	a.bridge.Emit("app:shutdown", map[string]any{})
	a.jsr.Stop()
	a.server.Stop(shutdownCtx)

	return nil
}

// Quit triggers a graceful shutdown: it unblocks the run loop, which closes all
// managed windows (CloseAll), runs OnShutdown, and stops the server. Safe to
// call from any goroutine — a bridge handler, an OS signal, or an ExitOnClose
// window closing — and idempotent (context cancellation is).
func (a *App) Quit() {
	if a.cancel != nil {
		a.cancel()
	}
}

// Stop is a deprecated alias for Quit.
func (a *App) Stop() { a.Quit() }

func (a *App) Invoke(name string, fn InvokeHandler) {
	a.bridge.Handle(name, fn)
}

func (a *App) On(event string, fn EventHandler) {
	a.bridge.On(event, fn)
}

func (a *App) Emit(event string, data any) {
	a.bridge.Emit(event, data)
}

// SetPolicy installs a capability ACL (see Policy) enforced on every invoke.
// Call before Run. Passing nil (the default) disables enforcement.
func (a *App) SetPolicy(p *Policy) {
	a.bridge.SetPolicy(p)
}

// OpenWindow opens an additional native window (a child process hosting one
// webview) and returns its id. Guarded: on platforms without native windowing
// (mobile, wasm/PWA) it returns an errors.ErrUnsupported-wrapped error rather
// than attempting to run. Available after Run has started the desktop app.
func (a *App) OpenWindow(opts WindowOptions) (int, error) {
	if err := requireWindowing(); err != nil {
		return 0, err
	}
	if a.windows == nil {
		return 0, fmt.Errorf("goleo: OpenWindow called before Run() started the desktop app")
	}
	return a.windows.Open(opts)
}

// CloseWindow closes the window with the given id. Guarded like OpenWindow.
func (a *App) CloseWindow(id int) error {
	if err := requireWindowing(); err != nil {
		return err
	}
	if a.windows == nil {
		return fmt.Errorf("goleo: CloseWindow called before Run() started the desktop app")
	}
	return a.windows.Close(id)
}

// ListWindows returns the ids of open managed windows. On platforms without
// windowing it returns an errors.ErrUnsupported-wrapped error.
func (a *App) ListWindows() ([]int, error) {
	if err := requireWindowing(); err != nil {
		return nil, err
	}
	if a.windows == nil {
		return nil, nil
	}
	return a.windows.List(), nil
}

// registerWindowCommands exposes multi-window control to the frontend via the
// bridge: goleo:windowOpen / windowClose / windowList.
func (a *App) registerWindowCommands() {
	// Handlers route through the guarded App methods so the bridge path returns
	// the same errors.ErrUnsupported as the Go API on unsupported platforms.
	a.bridge.Handle("goleo:windowOpen", func(ctx context.Context, args json.RawMessage) (any, error) {
		var opts WindowOptions
		if len(args) > 0 {
			if err := json.Unmarshal(args, &opts); err != nil {
				return nil, fmt.Errorf("invalid args: %w", err)
			}
		}
		id, err := a.OpenWindow(opts)
		if err != nil {
			return nil, err
		}
		return map[string]int{"id": id}, nil
	})

	a.bridge.Handle("goleo:windowClose", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
		return nil, a.CloseWindow(req.ID)
	})

	a.bridge.Handle("goleo:windowList", func(ctx context.Context, args json.RawMessage) (any, error) {
		ids, err := a.ListWindows()
		if err != nil {
			return nil, err
		}
		return map[string][]int{"ids": ids}, nil
	})

	a.bridge.Handle("goleo:quit", func(ctx context.Context, args json.RawMessage) (any, error) {
		a.Quit()
		return nil, nil
	})

	// Autostart (launch-on-login). App name from AppID, else Title.
	autoName := a.config.AppID
	if autoName == "" {
		autoName = a.config.Title
	}
	a.bridge.Handle("goleo:autostartEnable", func(ctx context.Context, args json.RawMessage) (any, error) {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		return nil, autostart.Enable(autoName, exe)
	})
	a.bridge.Handle("goleo:autostartDisable", func(ctx context.Context, args json.RawMessage) (any, error) {
		return nil, autostart.Disable(autoName)
	})
	a.bridge.Handle("goleo:autostartIsEnabled", func(ctx context.Context, args json.RawMessage) (any, error) {
		enabled, err := autostart.IsEnabled(autoName)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"enabled": enabled}, nil
	})
}
