// Headed, self-verifying check that the cgo-free glaze backend drives a live
// WKWebView (macOS) / WebKitGTK (Linux) window and completes a JS<->Go round-trip
// over glaze's Bind. Run on real hardware / a CI runner with a display (Linux
// needs xvfb); see .github/workflows/glaze-verify.yml and
// scripts/verify-linux-docker.*. "RESULT: PASS" + exit 0 on success.
//
// NOTE: this uses RAW glaze — it does NOT include goleo's WebKitGTK permission
// auto-grant shim (that lives in runtime/webview_glaze_permissions_linux.go), so
// it must NOT call getUserMedia/geolocation: on WebKitGTK an unanswered
// permission-request hangs the GTK main loop. Permission auto-grant is verified
// separately at the goleo-runtime level (an app built against the runtime, which
// includes the shim). Keeping this spike to a plain round-trip keeps it a clean,
// non-hanging signal of "glaze drives a live window + JS<->Go works".
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
(function send(){ if (window.report) { report("ok"); } else { setTimeout(send, 50); } })();
</script></body>`

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

	w, err := glaze.New(false)
	if err != nil {
		fmt.Println("RESULT: FAIL (glaze.New:", err, ")")
		os.Exit(1)
	}
	w.SetTitle("glaze verify")
	w.SetSize(420, 300, glaze.HintNone)

	var once sync.Once
	got := make(chan struct{})
	if err := w.Bind("report", func(msg string) {
		fmt.Println("JS->Go:", msg)
		once.Do(func() { close(got) })
	}); err != nil {
		fmt.Println("RESULT: FAIL (Bind:", err, ")")
		os.Exit(1)
	}

	w.Navigate("http://" + ln.Addr().String() + "/")

	go func() {
		select {
		case <-got:
		case <-time.After(20 * time.Second):
		}
		w.Terminate()
	}()

	w.Run()
	w.Destroy()

	select {
	case <-got:
		fmt.Println("RESULT: PASS (glaze drives a live window; JS<->Go round-trip)")
		os.Exit(0)
	default:
		fmt.Println("RESULT: FAIL (no JS->Go round-trip within timeout)")
		os.Exit(1)
	}
}
