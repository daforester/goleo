// Spike: is crgimenes/glaze a usable, cgo-free (CGO_ENABLED=0) webview binding
// for macOS (WKWebView) and Linux (WebKitGTK) — the pieces Goleo currently gets
// from the cgo webview/webview_go backend?
//
// This program exercises the glaze API surface that Goleo's WebviewWindow
// wrapper needs, so `CGO_ENABLED=0 GOOS=darwin|linux|windows go build` forces
// each per-OS glaze backend to compile. It does NOT open a window (headless CI /
// cross-compile check) — interactive UX still needs real hardware.
//
// Result (2026-07-12, from a Windows host): builds CGO_ENABLED=0 for
// darwin/{amd64,arm64}, linux/{amd64,arm64}, windows/amd64; runtime/cgo absent
// from every dependency tree; zero `import "C"` in glaze. See README.md.
package main

import (
	"fmt"

	"github.com/crgimenes/glaze"
)

func main() {
	w, err := glaze.New(false)
	if err != nil {
		// On a real desktop this creates the window+webview; under a headless
		// cross-compile smoke run we only care that it compiles and links.
		fmt.Println("glaze.New:", err)
		return
	}
	w.SetTitle("goleo")
	w.SetSize(800, 600, glaze.HintNone)
	w.Init("window.__goleo_native=1")
	w.Navigate("https://example.com")
	w.Eval("1+1")
	w.Dispatch(func() {})
	w.Terminate()
	w.Destroy()
}
