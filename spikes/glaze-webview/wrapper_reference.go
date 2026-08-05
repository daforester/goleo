package main

// Reference wrapper: how crgimenes/glaze would slot into Goleo's runtime as the
// cgo-free macOS/Linux webview backend, replacing the cgo webview/webview_go in
// runtime/webview.go. This mirrors runtime/webview_windows.go (which wraps
// jchv/go-webview2) almost line-for-line — glaze's WebView interface is the same
// New/Navigate/SetTitle/SetSize/Eval/Init/Bind/Run/Destroy/Dispatch/Terminate
// shape Goleo already wraps.
//
// In the real tree this becomes runtime/webview_darwin.go + runtime/webview_linux.go
// (identical bodies; split only so each can carry OS-specific permission wiring),
// using the runtime package's real windowConfig / nativeEvaler. The local copies
// below just let this file compile inside the standalone spike module.
//
// Migration checklist (the ~1 week of Phase-1 work this de-risks):
//   1. Add this as runtime/webview_darwin.go and runtime/webview_linux.go
//      (build tags `//go:build darwin && !mobilebuild` / `linux && ...`).
//   2. Delete runtime/webview.go (the cgo webview_go backend) and drop the
//      webview/webview_go dependency; keep webview_windows.go (go-webview2) OR
//      switch Windows to glaze too and delete both — glaze covers all three.
//   3. In cli/cmd/build.go, drop the `else CGO_ENABLED=1` branch so darwin/linux
//      also build CGO_ENABLED=0 (and thus cross-compile from any host).
//   4. Port webview_permissions_linux.go's auto-grant onto glaze's UI delegate
//      (or confirm glaze already grants camera/mic/geolocation).
//   5. Verify on real hardware: run Goleo's native-IPC {type,data} round-trip
//      (the Spike 2 test) through glaze's Bind against Bridge.HandleRequest.

import (
	"log"

	"github.com/crgimenes/glaze"
)

// --- stand-ins for the real runtime types (already defined in package runtime) ---

type windowConfig struct {
	Title           string
	Width, Height   int
	MinWidth        int
	MinHeight       int
	Center          bool
	URL             string
	DevTools        bool
	OnInit          func(*WebviewWindow)
	sessPlaceholder any // in runtime this is *nativeSession, stored on the window
}

// nativeEvaler is runtime/nativeipc.go's push interface; glaze.WebView satisfies
// it as-is (Dispatch(func()) + Eval(string)), so native IPC works unchanged.
type nativeEvaler interface {
	Dispatch(func())
	Eval(string)
}

// --- the wrapper (mirrors runtime/webview_windows.go) ---

type WebviewWindow struct {
	w   glaze.WebView
	cfg windowConfig
	url string
	// sess *nativeSession  // (in runtime) native IPC session
}

// NewWebviewWindow creates the window+webview. Note glaze.New returns an error
// (webview_go did not); the wrapper absorbs it so the runtime signature stays
// `func(windowConfig) WebviewWindow` — a nil w makes IsValid()/methods no-ops,
// matching how the mobile stub degrades.
func NewWebviewWindow(cfg windowConfig) WebviewWindow {
	w, err := glaze.New(cfg.DevTools)
	if err != nil {
		log.Printf("goleo webview (glaze): %v", err)
		return WebviewWindow{cfg: cfg}
	}
	w.SetTitle(cfg.Title)
	w.SetSize(cfg.Width, cfg.Height, glaze.HintNone)
	if cfg.MinWidth > 0 && cfg.MinHeight > 0 {
		w.SetSize(cfg.MinWidth, cfg.MinHeight, glaze.HintMin)
	}

	win := WebviewWindow{w: w, cfg: cfg, url: cfg.URL}

	// OnInit before Navigate — the native IPC shim + Bind must be registered
	// before the first page load (identical to webview_windows.go).
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

// evaler adapts to nativeipc.go's push interface — glaze.WebView already has
// Dispatch(func()) + Eval(string), so native IPC needs no per-backend changes.
func (win *WebviewWindow) evaler() nativeEvaler { return win.w }

// compile-time proof glaze.WebView satisfies the native IPC push interface.
var _ nativeEvaler = (glaze.WebView)(nil)
