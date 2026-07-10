//go:build !windows && !mobilebuild

// Non-Windows desktop (macOS, Linux) webview backend. This wraps
// github.com/webview/webview_go, which links the system webview via cgo
// (WebKitGTK on Linux, WKWebView on macOS). Windows uses a separate,
// cgo-free backend in webview_windows.go (WebView2 via COM/syscall), so this
// file is constrained to non-Windows targets.

package runtime

import (
	"fmt"
	"runtime"

	webview "github.com/webview/webview_go"
)

type WebviewWindow struct {
	w   webview.WebView
	cfg windowConfig
	url string
}

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

	if cfg.URL != "" {
		w.Navigate(cfg.URL)
	}

	return WebviewWindow{w: w, cfg: cfg, url: cfg.URL}
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

func (win *WebviewWindow) IsValid() bool {
	return win.w != nil
}

func init() {
	if runtime.GOOS == "linux" {
		fmt.Println("Goleo webview: using WebKitGTK (cgo)")
	} else if runtime.GOOS == "darwin" {
		fmt.Println("Goleo webview: using WKWebView (cgo)")
	}
}
