//go:build !mobilebuild

// Windows webview backend. Unlike the macOS/Linux backend (webview.go, which
// links the system webview through cgo), this uses github.com/jchv/go-webview2:
// a pure-Go binding to the WebView2 (Edge Chromium) runtime via COM + syscall.
// It has NO cgo dependency, so Windows desktop binaries build with
// CGO_ENABLED=0 and can be cross-compiled from any host — matching the
// CGO_ENABLED=0 flag that cli/cmd/build.go sets for desktop builds.
//
// The type name and method set are identical to the non-Windows WebviewWindow
// so callers (app.go, jsruntime.go, window_child.go) are platform-agnostic.

package runtime

import (
	"fmt"

	webview "github.com/jchv/go-webview2"
)

type WebviewWindow struct {
	w   webview.WebView
	cfg windowConfig
	url string
}

func NewWebviewWindow(cfg windowConfig) WebviewWindow {
	w := webview.NewWithOptions(webview.WebViewOptions{
		Debug:     cfg.DevTools,
		AutoFocus: true,
		WindowOptions: webview.WindowOptions{
			Title:  cfg.Title,
			Width:  uint(cfg.Width),
			Height: uint(cfg.Height),
			Center: cfg.Center,
		},
	})

	// WebView2 grants camera/mic/geolocation via a PermissionRequested event
	// rather than the WebKitGTK signal used on Linux (see
	// webview_permissions_linux.go). go-webview2 does not yet expose that hook,
	// so auto-grant is not wired here; the frontend's getUserMedia/geolocation
	// fallbacks will surface the WebView2 prompt instead. TODO: wire up
	// ICoreWebView2 PermissionRequested when the binding exposes it.

	if cfg.MinWidth > 0 && cfg.MinHeight > 0 {
		w.SetSize(cfg.MinWidth, cfg.MinHeight, webview.HintMin)
	}

	win := WebviewWindow{w: w, cfg: cfg, url: cfg.URL}

	// OnInit must run before the first navigation: init scripts and JS bindings
	// (e.g. the native IPC bridge) are only guaranteed to apply to pages loaded
	// after they are registered.
	if cfg.OnInit != nil {
		cfg.OnInit(&win)
	}

	if cfg.URL != "" {
		win.Navigate(cfg.URL)
	}

	return win
}

func (win *WebviewWindow) Navigate(url string) {
	if win.w != nil {
		win.w.Navigate(url)
		win.url = url
	}
}

func (win *WebviewWindow) SetTitle(title string) {
	if win.w != nil {
		win.w.SetTitle(title)
	}
}

func (win *WebviewWindow) SetSize(width, height int) {
	if win.w != nil {
		win.w.SetSize(width, height, webview.HintNone)
	}
}

func (win *WebviewWindow) Eval(js string) {
	if win.w != nil {
		win.w.Eval(js)
	}
}

// Init injects JavaScript that runs at the start of every page load, before the
// page's own scripts. Used to install the native IPC shim. Must be called
// before Navigate (see NewWebviewWindow / windowConfig.OnInit).
func (win *WebviewWindow) Init(js string) {
	if win.w != nil {
		win.w.Init(js)
	}
}

// Bind exposes a Go function as a global JS function of the given name (callable
// as window[name](...), returning a Promise). Used for the native IPC send
// channel. Must be called before Navigate.
func (win *WebviewWindow) Bind(name string, fn any) error {
	if win.w != nil {
		return win.w.Bind(name, fn)
	}
	return nil
}

func (win *WebviewWindow) Run() {
	if win.w != nil {
		win.w.Run()
	}
}

func (win *WebviewWindow) Destroy() {
	if win.w != nil {
		win.w.Destroy()
		win.w = nil
	}
}

// Dispatch runs f on the window's own UI thread (safe to call from any
// goroutine). Terminate ends the window's Run loop. Used by the in-process
// WindowManager to close a window from the backend goroutine.
func (win *WebviewWindow) Dispatch(f func()) {
	if win.w != nil {
		win.w.Dispatch(f)
	}
}

func (win *WebviewWindow) Terminate() {
	if win.w != nil {
		win.w.Terminate()
	}
}

func (win *WebviewWindow) IsValid() bool {
	return win.w != nil
}

func init() {
	fmt.Println("Goleo webview: using WebView2 (Edge Chromium, cgo-free)")
}
