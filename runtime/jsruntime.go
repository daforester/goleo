package runtime

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
)

type JSRuntime struct {
	vm     *goja.Runtime
	config Config
	app    *App
	port   int
	win    *WebviewWindow
	onStop []func()
}

func NewJSRuntime(cfg Config, app *App) *JSRuntime {
	vm := goja.New()
	return &JSRuntime{
		vm:     vm,
		config: cfg,
		app:    app,
	}
}

type windowConfig struct {
	Title     string
	Width     int
	Height    int
	MinWidth  int
	MinHeight int
	Center    bool
	URL       string
	DevTools  bool
	// OnInit, if set, runs against the window after the webview is created but
	// before its first navigation — the point at which init scripts and JS
	// bindings must be registered. Used to install the native IPC bridge
	// (see App.nativeOnInit); nil for windows that use the WebSocket transport.
	OnInit func(*WebviewWindow)

	// AssetScheme + AssetServe enable serving the window's UI from a portless,
	// secure custom origin (e.g. goleo://) instead of the loopback HTTP server.
	// Set only when Config.SchemeAssets is on and the backend supports it
	// (webviewSupportsSchemeAssets); AssetServe resolves a request path to bytes
	// + content type from the embedded FS. Empty AssetScheme = disabled.
	AssetScheme string
	AssetServe  func(urlPath string) ([]byte, string, bool)
}

// defaultInitCandidates are tried in order when Config.InitJS is not set.
// "init.js" matches the embedded key when main.go lives in backend/ and
// embeds its sibling init.js; "backend/init.js" matches on-disk dev runs
// started from the project root.
var defaultInitCandidates = []string{"init.js", "backend/init.js"}

// Run loads and executes the startup script. Resolution:
//   - Config.InitJS set: that file must exist (embedded or on disk) — an
//     error is returned if it cannot be loaded.
//   - Config.InitJS empty: init.js, then backend/init.js are tried; if none
//     exists Run returns nil and the app falls back to the built-in
//     Go-driven window setup from Config.
func (jsr *JSRuntime) Run() error {
	jsr.provideAPI()

	explicit := jsr.config.InitJS != ""
	candidates := defaultInitCandidates
	if explicit {
		candidates = []string{jsr.config.InitJS}
	}

	jsContent, path, err := jsr.loadInitScript(candidates)
	if err != nil {
		if explicit {
			return fmt.Errorf("loading init script %s: %w", jsr.config.InitJS, err)
		}
		// No default init script: use the built-in startup path.
		return nil
	}

	if _, err := jsr.vm.RunScript(path, jsContent); err != nil {
		return fmt.Errorf("executing %s: %w", path, err)
	}

	return nil
}

// loadInitScript tries each candidate against the embedded FS (production)
// and the working directory (dev mode), returning the first hit.
func (jsr *JSRuntime) loadInitScript(candidates []string) (content, path string, err error) {
	var attempts []string

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}

		if jsr.config.DevMode {
			for _, p := range []string{candidate, filepath.Join("..", candidate)} {
				data, readErr := os.ReadFile(p)
				if readErr == nil {
					return string(data), p, nil
				}
				attempts = append(attempts, p)
			}
		} else if jsr.config.EmbedFS != nil {
			c, readErr := readFileFromFS(jsr.config.EmbedFS, filepath.ToSlash(candidate))
			if readErr == nil {
				return c, candidate, nil
			}
			attempts = append(attempts, candidate+" (embedded)")
		}
	}

	return "", "", fmt.Errorf("no init script found (tried %s)", strings.Join(attempts, ", "))
}

func (jsr *JSRuntime) Stop() {
	for _, fn := range jsr.onStop {
		fn()
	}
}

func (jsr *JSRuntime) provideAPI() {
	jsr.vm.Set("createWindow", func(call goja.FunctionCall) goja.Value {
		// In browser mode (dev/emulation, mobile), don't create a native window
		if jsr.config.WindowMode == WindowModeBrowser {
			return jsr.vm.ToValue(false)
		}

		obj := call.Argument(0).ToObject(jsr.vm)

		cfg := windowConfig{
			Title:     getJSString(obj, "title", jsr.config.Title),
			Width:     getJSInt(obj, "width", jsr.config.Width),
			Height:    getJSInt(obj, "height", jsr.config.Height),
			MinWidth:  getJSInt(obj, "minWidth", 0),
			MinHeight: getJSInt(obj, "minHeight", 0),
			Center:    getJSBool(obj, "center", true),
			DevTools:  getJSBool(obj, "devTools", jsr.config.DevMode),
			URL:       getJSString(obj, "url", jsr.serverURL()),
			OnInit:    jsr.app.nativeOnInit(),
		}

		win := NewWebviewWindow(cfg)
		jsr.win = &win

		jsr.onStop = append(jsr.onStop, func() {
			win.Destroy()
		})

		return jsr.vm.ToValue(true)
	})

	jsr.vm.Set("getConfig", func(call goja.FunctionCall) goja.Value {
		cfg := jsr.config
		obj := jsr.vm.NewObject()
		obj.Set("title", cfg.Title)
		obj.Set("width", cfg.Width)
		obj.Set("height", cfg.Height)
		obj.Set("devMode", cfg.DevMode)
		obj.Set("devServer", cfg.DevServer)
		obj.Set("port", jsr.port)
		obj.Set("url", jsr.serverURL())
		return obj
	})

	console := jsr.vm.NewObject()
	logFn := func(prefix string) func(call goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			parts := make([]string, len(call.Arguments))
			for i, arg := range call.Arguments {
				parts[i] = arg.String()
			}
			log.Printf("[init.js]%s %s", prefix, strings.Join(parts, " "))
			return goja.Undefined()
		}
	}
	console.Set("log", logFn(""))
	console.Set("info", logFn(""))
	console.Set("warn", logFn(" WARN:"))
	console.Set("error", logFn(" ERROR:"))
	jsr.vm.Set("console", console)
}

// serverURL is the address the window should load: the Vite dev server in
// dev mode, otherwise the embedded HTTP server.
func (jsr *JSRuntime) serverURL() string {
	if jsr.app != nil {
		return jsr.app.serverURL(jsr.port)
	}
	if jsr.config.DevMode {
		if jsr.config.DevServer != "" {
			return jsr.config.DevServer
		}
		return "http://localhost:5173"
	}
	return fmt.Sprintf("http://localhost:%d", jsr.port)
}

func getJSString(obj *goja.Object, key, def string) string {
	v := obj.Get(key)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return def
	}
	return v.String()
}

func getJSInt(obj *goja.Object, key string, def int) int {
	v := obj.Get(key)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return def
	}
	return int(v.ToInteger())
}

func getJSBool(obj *goja.Object, key string, def bool) bool {
	v := obj.Get(key)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return def
	}
	return v.ToBoolean()
}

func readFileFromFS(embedFS any, path string) (string, error) {
	fs, ok := embedFS.(interface {
		ReadFile(name string) ([]byte, error)
	})
	if ok {
		data, err := fs.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return "", fmt.Errorf("embed FS does not support ReadFile")
}
