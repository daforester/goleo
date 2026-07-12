// Headed, self-verifying check that the cgo-free glaze backend drives a live
// WKWebView (macOS) / WebKitGTK (Linux) window AND that permission requests are
// auto-granted, so getUserMedia resolves instead of hanging/being denied.
//
// Two things are proven:
//  1. JS<->Go round-trip over glaze's Bind (report(...)).
//  2. Permission gate: getUserMedia({video:true}) over a secure 127.0.0.1 origin
//     must get PAST the WebKit permission prompt. On a headless CI runner there
//     is no camera, so it typically rejects with NotFoundError/NotReadableError
//     — that still proves the permission was GRANTED. Only NotAllowedError means
//     the grant failed (the shim didn't work).
//
// A plain SetHtml/about:blank page is NOT a secure context, so getUserMedia
// would be unavailable regardless of permissions — hence we serve over
// http://127.0.0.1 (a potentially-trustworthy origin).
//
// Run on real hardware / a CI runner with a display (Linux needs xvfb); see
// .github/workflows/glaze-verify.yml. Exits 0 + "RESULT: PASS" on success.
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
function send(p){ if (window.report) { report(p); } else { setTimeout(function(){ send(p); }, 50); } }
send("native-ok");
if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
  send("perm-NoMediaDevices");
} else {
  navigator.mediaDevices.getUserMedia({ video: true })
    .then(function(s){ s.getTracks().forEach(function(t){ t.stop(); }); send("perm-ok"); })
    .catch(function(e){ send("perm-" + (e && e.name ? e.name : "Unknown")); });
}
</script></body>`

func main() {
	// Serve the test page over a loopback HTTP origin (secure context).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("RESULT: FAIL (listen:", err, ")")
		os.Exit(1)
	}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	}))
	url := "http://" + ln.Addr().String() + "/"

	w, err := glaze.New(false)
	if err != nil {
		fmt.Println("RESULT: FAIL (glaze.New:", err, ")")
		os.Exit(1)
	}
	w.SetTitle("glaze verify")
	w.SetSize(420, 300, glaze.HintNone)

	var mu sync.Mutex
	got := map[string]bool{}
	if err := w.Bind("report", func(phase string) {
		fmt.Println("JS->Go:", phase)
		mu.Lock()
		got[phase] = true
		done := got["native-ok"] && hasPermVerdict(got)
		mu.Unlock()
		if done {
			w.Terminate()
		}
	}); err != nil {
		fmt.Println("RESULT: FAIL (Bind:", err, ")")
		os.Exit(1)
	}

	w.Navigate(url)

	go func() {
		time.Sleep(25 * time.Second)
		w.Terminate()
	}()

	w.Run()
	w.Destroy()

	mu.Lock()
	defer mu.Unlock()
	roundTrip := got["native-ok"]
	permGranted := got["perm-ok"] || got["perm-NotFoundError"] || got["perm-NotReadableError"]
	permDenied := got["perm-NotAllowedError"]

	switch {
	case roundTrip && permGranted:
		fmt.Println("RESULT: PASS (round-trip + permission auto-grant on real hardware)")
		os.Exit(0)
	case roundTrip && permDenied:
		fmt.Println("RESULT: FAIL (permission DENIED — the auto-grant shim did not work)")
		os.Exit(1)
	case roundTrip:
		fmt.Printf("RESULT: INCONCLUSIVE (round-trip ok; getUserMedia verdict=%v — check WebKit media/secure-context support)\n", keys(got))
		os.Exit(2)
	default:
		fmt.Println("RESULT: FAIL (no JS->Go round-trip within timeout)")
		os.Exit(1)
	}
}

// hasPermVerdict reports whether getUserMedia has produced any terminal outcome,
// so we can stop the loop instead of waiting for the full timeout.
func hasPermVerdict(got map[string]bool) bool {
	for k := range got {
		if len(k) > 5 && k[:5] == "perm-" {
			return true
		}
	}
	return got["perm-ok"]
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
