// Headed, self-verifying JS<->Go round-trip over crgimenes/glaze — the real
// (non-headless) check that the cgo-free glaze backend actually drives a live
// WKWebView (macOS) / WebKitGTK (Linux) window, not just that it compiles.
//
// This is the hardware-gated companion to ../main.go (which only cross-compiles).
// Run it on a real desktop or a CI runner with a display (see
// .github/workflows/glaze-verify.yml; Linux needs xvfb). Exits 0 + prints
// "RESULT: PASS" on a completed round-trip, non-zero otherwise. Mirrors the
// WebView2 smoke Goleo already ran on Windows and the Spike 2 macOS round-trip.
package main

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/crgimenes/glaze"
)

func main() {
	w, err := glaze.New(false)
	if err != nil {
		fmt.Println("RESULT: FAIL (glaze.New:", err, ")")
		os.Exit(1)
	}
	w.SetTitle("glaze verify")
	w.SetSize(420, 300, glaze.HintNone)

	var passed atomic.Bool

	// JS -> Go: the page calls report("ok"); receiving it proves the bound Go
	// function and the message channel work.
	if err := w.Bind("report", func(msg string) {
		fmt.Println("JS->Go:", msg)
		if msg == "ok" {
			passed.Store(true)
		}
		w.Terminate()
	}); err != nil {
		fmt.Println("RESULT: FAIL (Bind:", err, ")")
		os.Exit(1)
	}

	// Load a page that calls back into Go once the binding is present.
	w.SetHtml(`<!doctype html><meta charset="utf-8"><body>
<script>
(function go(){ if (window.report) { report("ok"); } else { setTimeout(go, 50); } })();
</script></body>`)

	// Safety net so CI never hangs if the round-trip never completes.
	go func() {
		time.Sleep(20 * time.Second)
		w.Terminate()
	}()

	w.Run()
	w.Destroy()

	if passed.Load() {
		fmt.Println("RESULT: PASS (glaze JS<->Go round-trip on real hardware)")
		os.Exit(0)
	}
	fmt.Println("RESULT: FAIL (no round-trip within timeout)")
	os.Exit(1)
}
