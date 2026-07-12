//go:build (darwin || linux) && !mobilebuild && goleo_glaze

// Opt-in cgo-free macOS/Linux webview backend (build tag `goleo_glaze`).
//
// This replaces the default cgo webview_go backend (webview.go) with
// github.com/crgimenes/glaze — a purego reimplementation of WKWebView (macOS)
// and WebKitGTK (Linux) — so darwin/linux desktop binaries build with
// CGO_ENABLED=0 and cross-compile from any host, exactly like the Windows
// WebView2 backend (webview_windows.go).
//
// Status: cross-compile-verified cgo-free (see spikes/glaze-webview); NOT yet
// validated on real macOS/Linux hardware, which is why it is opt-in behind
// `goleo_glaze` rather than the default. Flip the default (drop webview.go, make
// this unconditional) once the hardware round-trip passes. glaze's WebView
// interface matches webview_windows.go's, so this wrapper is intentionally
// near-identical to it.

package runtime

import (
	"log"

	"github.com/crgimenes/glaze"
)

// (permission auto-grant lives in webview_glaze_permissions_{linux,other}.go)

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
	w, err := glaze.New(cfg.DevTools)
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

func (win *WebviewWindow) IsValid() bool { return win.w != nil }
