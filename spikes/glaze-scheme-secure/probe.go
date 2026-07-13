// Spike: does serving an app's UI from a custom, portless origin still give a
// SECURE CONTEXT on each desktop webview backend?
//
// Native IPC already removed the RPC/WebSocket surface; the only remaining
// reason a Goleo desktop app opens a loopback TCP port is to serve its embedded
// assets over http://127.0.0.1 — which the browser treats as a secure context,
// so localStorage / crypto.subtle / getUserMedia / history routing all work.
//
// A "goleo://" custom scheme would remove that last port. But serving bytes is
// not the hard part — the hard part is whether the custom origin is a SECURE
// CONTEXT. This spike answers exactly that, per platform, with the lightest
// cgo-free mechanism each backend offers:
//
//   macOS   (main_darwin.go)  raw purego/objc WKWebView + WKURLSchemeHandler
//                             (there is NO public "register scheme as secure"
//                             API — this is the gating unknown)
//   Linux   (main_linux.go)   glaze + an external purego shim that registers the
//                             scheme via webkit_security_manager_*_as_secure
//   Windows (main_windows.go) go-webview2 edge.Chromium +
//                             SetVirtualHostNameToFolderMapping over https://
//
// Each loads the SAME probe page (below) from the custom origin; the page reports
// isSecureContext + a real localStorage write + a real crypto.subtle.digest back
// to Go. PASS on a platform == the "goleo://" PR is viable there. macOS decides
// whether the uniform, all-platforms PR is possible at all.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// probeHTML is served from the custom origin on every platform. send() adapts to
// whichever Go<-JS channel the backend provides; the probe itself is identical.
const probeHTML = `<!doctype html><html><head><meta charset="utf-8"><title>secure-probe</title></head>
<body><h1>secure-context probe</h1>
<script>
function send(s){
  try {
    if (window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.report) {
      window.webkit.messageHandlers.report.postMessage(s);            // macOS WKScriptMessageHandler
    } else if (typeof report === "function") {
      report(s);                                                       // glaze / go-webview2 Bind
    } else if (window.chrome && window.chrome.webview) {
      window.chrome.webview.postMessage(s);                            // WebView2 postMessage
    }
  } catch (e) { /* nothing we can do to report the failure to report */ }
}
(async function(){
  var r = { origin: location.origin, secure: (window.isSecureContext === true), ls:false, crypto:false };
  try {
    localStorage.setItem("goleo_probe","1");
    r.ls = (localStorage.getItem("goleo_probe") === "1");
    localStorage.removeItem("goleo_probe");
  } catch(e){ r.lsErr = String(e); }
  try {
    if (window.crypto && window.crypto.subtle) {
      var d = await window.crypto.subtle.digest("SHA-256", new TextEncoder().encode("goleo"));
      r.crypto = (d.byteLength === 32);
    } else {
      r.cryptoErr = "crypto.subtle undefined (non-secure context)";
    }
  } catch(e){ r.cryptoErr = String(e); }
  send(JSON.stringify(r));
})();
</script></body></html>`

// probeResult mirrors the JSON the probe page posts back.
type probeResult struct {
	Origin    string `json:"origin"`
	Secure    bool   `json:"secure"`
	LS        bool   `json:"ls"`
	Crypto    bool   `json:"crypto"`
	LSErr     string `json:"lsErr"`
	CryptoErr string `json:"cryptoErr"`
}

// reportResult parses one report payload, prints a human-readable breakdown and
// the machine-greppable RESULT: line, and returns whether this platform passes.
// PASS requires a secure context AND working localStorage AND working WebCrypto —
// the three capabilities the loopback http://127.0.0.1 origin gives today.
func reportResult(backend, jsonStr string) bool {
	var r probeResult
	if err := json.Unmarshal([]byte(jsonStr), &r); err != nil {
		fmt.Printf("RESULT: FAIL (%s) — could not parse probe report: %v; raw=%q\n", backend, err, jsonStr)
		return false
	}
	fmt.Printf("[probe] backend=%s origin=%q isSecureContext=%v localStorage=%v crypto.subtle=%v\n",
		backend, r.Origin, r.Secure, r.LS, r.Crypto)
	if r.LSErr != "" {
		fmt.Printf("[probe]   localStorage error: %s\n", r.LSErr)
	}
	if r.CryptoErr != "" {
		fmt.Printf("[probe]   crypto.subtle error: %s\n", r.CryptoErr)
	}
	pass := r.Secure && r.LS && r.Crypto
	if pass {
		fmt.Printf("RESULT: PASS (%s) — custom origin %q is a secure context (localStorage + WebCrypto work)\n", backend, r.Origin)
	} else {
		fmt.Printf("RESULT: FAIL (%s) — custom origin %q is NOT a full secure context (secure=%v ls=%v crypto=%v)\n",
			backend, r.Origin, r.Secure, r.LS, r.Crypto)
	}
	return pass
}

func exitFromResult(pass bool) {
	if pass {
		os.Exit(0)
	}
	os.Exit(1)
}
