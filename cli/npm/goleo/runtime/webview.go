//go:build (darwin || linux) && !mobilebuild && goleo_cgo_webview

// Legacy cgo macOS/Linux webview backend (opt-in, build tag `goleo_cgo_webview`).
// Wraps github.com/webview/webview_go, which links the system webview via cgo
// (WebKitGTK on Linux, WKWebView on macOS) and thus needs CGO_ENABLED=1 + the
// platform toolchain. The DEFAULT macOS/Linux backend is now the cgo-free
// glaze wrapper (webview_glaze.go); this is kept one release as a fallback.
// Windows always uses the cgo-free WebView2 backend (webview_windows.go).

package runtime

import (
	"fmt"
	"runtime"
	"unsafe"

	webview "github.com/webview/webview_go"
)

type WebviewWindow struct {
	w    webview.WebView
	cfg  windowConfig
	url  string
	sess *nativeSession // native IPC session (Config.NativeIPC); nil otherwise
}

// evaler adapts the backend to the native IPC push interface (Dispatch + Eval).
func (win *WebviewWindow) evaler() nativeEvaler { return win.w }

// webviewSupportsSchemeAssets: this backend does not (yet) serve the UI from a
// custom secure scheme, so Config.SchemeAssets falls back to the loopback server.
func webviewSupportsSchemeAssets() bool { return false }

func NewWebviewWindow(cfg windowConfig) WebviewWindow {
	debug := cfg.DevTools

	w := webview.New(debug)
	// Auto-grant OS permission requests (camera, mic, geolocation) so the
	// frontend's browser-API fallbacks resolve instead of hanging the webview.
	// No-op on non-Linux desktops (see webview_permissions_*.go).
	enableWebviewPermissions(w.Window())
	w.SetTitle(cfg.Title)
	w.SetSize(cfg.Width, cfg.Height, webview.HintNone)

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

// Dispatch runs f on the window's own UI thread; Terminate ends its Run loop.
// Used by the in-process WindowManager to close a window from another goroutine.
func (win *WebviewWindow) Dispatch(f func()) {
	if win.w != nil {
		win.w.Dispatch(f)
	}
}

// endRunLoop unblocks App.Run at shutdown (cgo webview_go: Terminate ends the loop).
func (win *WebviewWindow) endRunLoop() { win.Terminate() }

func (win *WebviewWindow) Terminate() {
	if win.w != nil {
		win.w.Terminate()
	}
}

func (win *WebviewWindow) IsValid() bool {
	return win.w != nil
}

// NativeHandle returns the OS window handle (GtkWindow*/NSWindow* via the cgo
// backend), used by the native menu-bar backend. Nil if not created.
func (win *WebviewWindow) NativeHandle() unsafe.Pointer {
	if win.w == nil {
		return nil
	}
	return win.w.Window()
}

func init() {
	if runtime.GOOS == "linux" {
		fmt.Println("Goleo webview: using WebKitGTK (cgo)")
	} else if runtime.GOOS == "darwin" {
		fmt.Println("Goleo webview: using WKWebView (cgo)")
	}
}
