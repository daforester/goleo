# glaze issue draft (custom URL-scheme handlers)

**Internal (do not post):** Per glaze's CONTRIBUTING, a new API needs an **issue
first** to agree it fits scope + the two rules before the PR. Post only the block
between the `POST FROM HERE` / `END POST` markers. The reference implementation
lives in `spikes/glaze-scheme-secure/glazefork/` and is verified on real hardware
(see `README.md`); the paste-ready PR body is in `PR_DESCRIPTION.md`.

---
<!-- ===================== POST FROM HERE ===================== -->

**Title:** Add a custom URL-scheme handler API (`NewWithOptions` / `Options.SchemeHandlers`)

### Problem
A glaze desktop app that loads a real frontend has to serve it from a local
`http://127.0.0.1:<port>` server, because the engine treats loopback as a **secure
context** — and `localStorage`, `crypto.subtle`, `getUserMedia`, and history-based
routing are all gated behind a secure context. The alternatives (`file://`,
`SetHtml`) are **not** secure contexts, so they break those APIs.

There's no way today to get **no port *and* a secure context**. The fix every
native engine supports is a **custom scheme registered as secure**, serving
embedded assets in-process — but glaze exposes no hook for it.

### Why this fits glaze
- **No CGo (rule 1).** Implemented with **purego / syscall / the Obj-C runtime /
  COM only** — no `import "C"`, **no new dependency** (purego stays glaze's only
  dep), `CGO_ENABLED=0` green for darwin/{arm64,amd64}, linux/{amd64,arm64},
  windows/{amd64,arm64}.
- **YAGNI (rule 2).** Smallest API that works: two 1- and 2-field structs and one
  constructor; no speculative fields (no status/headers/method until a real case
  needs them). `New`/`NewWindow` just delegate, so nothing existing changes.
- **In scope.** This is the webview binding serving *its own* content — core to
  what glaze is, not a window-independent OS binding (nothing for the `native`
  project).
- **All three platforms**, each via its engine's own mechanism (below).

### Proposed API
Additive and backward-compatible:

```go
type SchemeRequest struct { URL string }
type SchemeResponse struct {
    Body     []byte
    MIMEType string // default application/octet-stream
} // a nil response is treated as "not found"
type SchemeHandler func(*SchemeRequest) *SchemeResponse

type Options struct {
    Debug          bool
    Window         unsafe.Pointer
    SchemeHandlers map[string]SchemeHandler
}

func NewWithOptions(opts Options) (WebView, error)
// New(debug)          == NewWithOptions(Options{Debug: debug})
// NewWindow(debug, w) == NewWithOptions(Options{Debug: debug, Window: w})
```

Handlers are install-time only (macOS copies the `WKWebViewConfiguration` at init),
so they're an `Options` field, not a method on the running `WebView`.

### Per-backend mechanism
- **macOS (WKWebView):** a `WKURLSchemeHandler` set on the configuration **before**
  the view is created (the config is copied at init — the reason this must live in
  glaze).
- **Linux (WebKitGTK):** `webkit_web_context_register_uri_scheme` +
  `webkit_security_manager_register_uri_scheme_as_secure`, served from an in-memory
  `GInputStream`.
- **Windows (WebView2):** WebView2 has no per-scheme secure flag, so a scheme is
  served over a per-scheme `https://<scheme>.localhost` virtual host (an https
  origin is a secure context) via `AddWebResourceRequestedFilter` +
  `WebResourceRequested`, answered in memory; `Navigate` rewrites `<scheme>://…` to
  the vhost so callers use one scheme URL everywhere. (A literal `app://` is
  possible via `CoreWebView2CustomSchemeRegistration`, but that needs inbound COM
  env-options objects — more code for a cosmetic origin match; happy to do it if
  you'd prefer it.)

### Verified (real hardware)
A page served from a custom scheme reports `window.isSecureContext === true` with
working `localStorage` + `crypto.subtle` on **WKWebView (macOS 14)**, **WebKitGTK
(GTK3 + GTK4)**, and **WebView2 (Windows)**. `gofmt`/`go vet` clean; `go test
-short` passes (a headless test covers the URL rewrite); audited `unsafe`/`uintptr`
marked `// #nosec Gxxx`.

**Would you accept a PR along these lines, and do you have a preference on the API
shape** (the `Options` map vs. a `RegisterScheme` builder; the response model; the
Windows vhost vs. `CustomSchemeRegistration`)?

<!-- ===================== END POST ===================== -->
---

**Internal — goleo consumption (do not post):** goleo ships this via a pinned fork
(`scripts/pin-glaze-fork.*`), so upstream merge is cleanup, not a blocker. goleo
consumes the scheme API on macOS + Linux (`runtime/webview_glaze.go`,
`Config.SchemeAssets`), verified end-to-end on `macos-14` + Linux GTK3/GTK4;
Windows still wraps `jchv/go-webview2` (loopback fallback) until the separate
Windows→glaze migration, after which glaze's Windows scheme wiring is consumed too.
