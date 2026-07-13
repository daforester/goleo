//go:build (darwin || linux || windows) && !mobilebuild && !goleo_cgo_webview

// Default cgo-free webview backend for ALL three desktops (macOS, Linux, Windows).
//
// Uses github.com/crgimenes/glaze — a purego reimplementation of WKWebView
// (macOS), WebKitGTK (Linux) and WebView2 (Windows) behind one interface — so
// every desktop binary builds CGO_ENABLED=0 and cross-compiles from any host, and
// goleo carries ONE webview binding instead of two. Verified cgo-free
// (spikes/glaze-webview) and on real macOS + Linux + Windows hardware
// (glaze-verify.yml + local Windows runs: JS<->Go round-trip, native IPC, custom
// scheme assets, in-process multi-window).
//
// Fallback: the macOS/Linux cgo webview_go backend (webview.go) remains behind
// `-tags goleo_cgo_webview` for one release. (The Windows go-webview2 backend has
// been removed — glaze is the sole Windows path now.)

package runtime

import (
	"log"
	"unsafe"

	"github.com/crgimenes/glaze"
)

// (permission auto-grant lives in webview_glaze_permissions_{linux,other}.go)

// webviewSupportsSchemeAssets reports that this backend can serve the UI from a
// custom secure scheme (glaze.Options.SchemeHandlers) — true for WKWebView +
// WebKitGTK. See Config.SchemeAssets.
func webviewSupportsSchemeAssets() bool { return true }

type WebviewWindow struct {
	w    glaze.WebView
	cfg  windowConfig
	url  string
	sess *nativeSession // native IPC session (Config.NativeIPC); nil otherwise
}

// evaler adapts the backend to the native IPC push interface (Dispatch + Eval).
// glaze.WebView satisfies nativeEvaler as-is.
func (win *WebviewWindow) evaler() nativeEvaler { return win.w }

func NewWebviewWindow(cfg windowConfig) WebviewWindow {
	w, err := newGlazeWebView(cfg)
	if err != nil {
		// Degrade like the mobile stub: a nil backend makes every method a
		// guarded no-op and IsValid() false, rather than crashing the caller.
		log.Printf("Goleo webview (glaze): %v", err)
		return WebviewWindow{cfg: cfg}
	}

	// Auto-grant WebKitGTK permission requests (camera/mic/geolocation) so the
	// app's getUserMedia/geolocation fallbacks resolve instead of hanging. No-op
	// off Linux (macOS handles it via the WKUIDelegate).
	enableGlazePermissions(w.Window())

	w.SetTitle(cfg.Title)
	w.SetSize(cfg.Width, cfg.Height, glaze.HintNone)
	if cfg.MinWidth > 0 && cfg.MinHeight > 0 {
		w.SetSize(cfg.MinWidth, cfg.MinHeight, glaze.HintMin)
	}

	win := WebviewWindow{w: w, cfg: cfg, url: cfg.URL}

	// OnInit must run before the first navigation (native IPC shim + Bind).
	if cfg.OnInit != nil {
		cfg.OnInit(&win)
	}
	if cfg.URL != "" {
		win.Navigate(cfg.URL)
	}
	return win
}

// newGlazeWebView creates the glaze web view, wiring a custom asset scheme when
// the window is configured for one (Config.SchemeAssets). Without it, this is the
// plain glaze.New the backend used before.
func newGlazeWebView(cfg windowConfig) (glaze.WebView, error) {
	if cfg.AssetScheme == "" || cfg.AssetServe == nil {
		return glaze.New(cfg.DevTools)
	}
	serve := cfg.AssetServe
	return glaze.NewWithOptions(glaze.Options{
		Debug: cfg.DevTools,
		SchemeHandlers: map[string]glaze.SchemeHandler{
			cfg.AssetScheme: func(req *glaze.SchemeRequest) *glaze.SchemeResponse {
				body, ct, ok := serve(req.URL)
				if !ok {
					return nil // nil response => not found
				}
				return &glaze.SchemeResponse{Body: body, MIMEType: ct}
			},
		},
	})
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
		win.w.SetSize(width, height, glaze.HintNone)
	}
}

func (win *WebviewWindow) Eval(js string) {
	if win.w != nil {
		win.w.Eval(js)
	}
}

func (win *WebviewWindow) Init(js string) {
	if win.w != nil {
		win.w.Init(js)
	}
}

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

// endRunLoop unblocks App.Run's blocking Run() at shutdown. glaze's Terminate is
// safe to call from any goroutine and stops the shared run loop (all windows) on
// every desktop, so Run returns; Destroy would only close the primary window.
func (win *WebviewWindow) endRunLoop() { win.Terminate() }

func (win *WebviewWindow) IsValid() bool { return win.w != nil }

// NativeHandle returns the OS window handle — GtkWindow* on Linux, NSWindow* on
// macOS, HWND on Windows — used by the native menu-bar backend. Nil if the window
// isn't created.
func (win *WebviewWindow) NativeHandle() unsafe.Pointer {
	if win.w == nil {
		return nil
	}
	return win.w.Window()
}
