//go:build darwin || linux || windows

// Proof that the PROPOSED glaze API (glaze.NewWithOptions + Options.SchemeHandlers)
// delivers a secure context on macOS and Linux — i.e. the exact change goleo
// would consume from a glaze fork, exercised through glaze's own architecture
// (config/init flow, Bind, run loop), not the raw purego of the sibling spike.
//
// macOS is the one that matters: it's the only platform where the scheme handler
// MUST live inside glaze (config frozen at init). Runs on macos-14 via
// glaze-verify.yml; Linux runs under Docker/xvfb. (Windows is excluded — goleo
// uses jchv/go-webview2 there; glaze's WebView2 scheme support is an upstream TODO.)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/crgimenes/glaze"
)

func init() { runtime.LockOSThread() }

const scheme = "goleoapp"

const probeHTML = `<!doctype html><html><head><meta charset="utf-8"><title>secure-probe</title></head>
<body><h1>glaze scheme probe</h1>
<script>
(async function(){
  var r = { origin: location.origin, secure: (window.isSecureContext === true), ls:false, crypto:false };
  try { localStorage.setItem("k","1"); r.ls = (localStorage.getItem("k")==="1"); localStorage.removeItem("k"); }
  catch(e){ r.lsErr = String(e); }
  try {
    if (window.crypto && window.crypto.subtle) {
      var d = await window.crypto.subtle.digest("SHA-256", new TextEncoder().encode("goleo"));
      r.crypto = (d.byteLength === 32);
    } else { r.cryptoErr = "crypto.subtle undefined"; }
  } catch(e){ r.cryptoErr = String(e); }
  report(JSON.stringify(r));   // bound Go function (glaze.Bind)
})();
</script></body></html>`

type probeResult struct {
	Origin    string `json:"origin"`
	Secure    bool   `json:"secure"`
	LS        bool   `json:"ls"`
	Crypto    bool   `json:"crypto"`
	LSErr     string `json:"lsErr"`
	CryptoErr string `json:"cryptoErr"`
}

var (
	gotReport  string
	haveReport bool
)

func main() {
	fmt.Fprintln(os.Stderr, "[spike] glaze NewWithOptions scheme-handler secure-context probe")

	wv, err := glaze.NewWithOptions(glaze.Options{
		SchemeHandlers: map[string]glaze.SchemeHandler{
			scheme: func(*glaze.SchemeRequest) *glaze.SchemeResponse {
				return &glaze.SchemeResponse{Body: []byte(probeHTML), MIMEType: "text/html"}
			},
		},
	})
	if err != nil {
		fmt.Println("RESULT: FAIL (glaze) — NewWithOptions:", err)
		os.Exit(1)
	}
	wv.SetTitle("glaze scheme spike")
	wv.SetSize(640, 480, glaze.HintNone)

	if err := wv.Bind("report", func(s string) {
		gotReport, haveReport = s, true
		wv.Terminate()
	}); err != nil {
		fmt.Println("RESULT: FAIL (glaze) — Bind:", err)
		os.Exit(1)
	}

	wv.Navigate(scheme + "://app/index.html")
	wv.Run()

	if !haveReport {
		fmt.Println("RESULT: FAIL (glaze) — no probe report (scheme handler may not have fired)")
		os.Exit(1)
	}
	var r probeResult
	if err := json.Unmarshal([]byte(gotReport), &r); err != nil {
		fmt.Printf("RESULT: FAIL (glaze) — bad report %q: %v\n", gotReport, err)
		os.Exit(1)
	}
	fmt.Printf("[probe] via glaze API: origin=%q isSecureContext=%v localStorage=%v crypto.subtle=%v\n",
		r.Origin, r.Secure, r.LS, r.Crypto)
	if r.CryptoErr != "" {
		fmt.Printf("[probe]   crypto.subtle error: %s\n", r.CryptoErr)
	}
	if r.Secure && r.LS && r.Crypto {
		fmt.Printf("RESULT: PASS (glaze NewWithOptions) — %q is a secure context via the glaze scheme API\n", r.Origin)
		os.Exit(0)
	}
	fmt.Printf("RESULT: FAIL (glaze) — %q not a full secure context (secure=%v ls=%v crypto=%v)\n",
		r.Origin, r.Secure, r.LS, r.Crypto)
	os.Exit(1)
}
