// Spike: can glaze host TWO live windows under ONE run loop, in one process —
// the "single-loop master" model macOS needs (AppKit is main-thread-only, so
// goleo's Windows goroutine-per-window trick can't work there)?
//
// glaze's darwin backend shares one NSApplication and tracks windowCount
// (terminating only when the last window closes), so extra windows should be
// creatable on the main thread while the primary's [NSApp run] loop is live.
// This program proves the DYNAMIC case goleo needs: open the 2nd window AFTER
// the loop is running, via Dispatch onto the main thread, and confirm BOTH
// windows load and complete a JS->Go round-trip. Never calls Run() on window 2.
//
// Runs the same on Linux (GTK is likewise main-thread-only) — glaze's
// windowCount logic is cross-platform. Cross-compiles cgo-free; run on real
// hardware / a CI runner with a display (Linux needs xvfb). "RESULT: PASS" +
// exit 0 on success.
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/crgimenes/glaze"
)

const page = `<!doctype html><meta charset="utf-8"><body><script>
var w = new URLSearchParams(location.search).get('win') || '?';
(function send(){ if (window.report) { report(w); } else { setTimeout(send, 50); } })();
</script></body>`

// win2 is kept referenced so its native window isn't GC'd out from under us.
var win2 glaze.WebView

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("RESULT: FAIL (listen:", err, ")")
		os.Exit(1)
	}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	}))
	base := "http://" + ln.Addr().String()

	primary, err := glaze.New(false)
	if err != nil {
		fmt.Println("RESULT: FAIL (glaze.New primary:", err, ")")
		os.Exit(1)
	}
	primary.SetTitle("win1")
	primary.SetSize(400, 300, glaze.HintNone)

	var (
		mu       sync.Mutex
		got      = map[string]bool{}
		openOnce sync.Once
		doneOnce sync.Once
	)

	var bindReport func(glaze.WebView)
	bindReport = func(w glaze.WebView) {
		_ = w.Bind("report", func(id string) {
			fmt.Println("window ready:", id)
			mu.Lock()
			got[id] = true
			have1, have2 := got["1"], got["2"]
			mu.Unlock()

			// Window 1 is live — now open window 2 on the main thread while the
			// primary run loop is already going (the dynamic case goleo needs).
			if id == "1" {
				openOnce.Do(func() {
					primary.Dispatch(func() {
						w2, e := glaze.New(false)
						if e != nil {
							fmt.Println("RESULT: FAIL (glaze.New window2:", e, ")")
							return
						}
						w2.SetTitle("win2")
						w2.SetSize(400, 300, glaze.HintNone)
						bindReport(w2)
						w2.Navigate(base + "/?win=2")
						win2 = w2
					})
				})
			}
			if have1 && have2 {
				doneOnce.Do(func() {
					go func() { time.Sleep(400 * time.Millisecond); primary.Terminate() }()
				})
			}
		})
	}

	bindReport(primary)
	primary.Navigate(base + "/?win=1")

	go func() { time.Sleep(30 * time.Second); primary.Terminate() }()

	primary.Run() // single NSApp/GTK loop serving BOTH windows

	mu.Lock()
	ok := got["1"] && got["2"]
	mu.Unlock()
	if ok {
		fmt.Println("RESULT: PASS (two windows, one run loop, both round-tripped)")
		os.Exit(0)
	}
	fmt.Printf("RESULT: FAIL (windows ready: %v)\n", got)
	os.Exit(1)
}
