# Draft issue/PR text for github.com/crgimenes/glaze

Post the **issue** first to align on the API shape before sending the PR (see
"Fork + upstream" note at the bottom). The working implementation for macOS +
Linux lives in `spikes/glaze-scheme-secure/glazefork/` (a copy of glaze v0.0.31
with the change) and is verified on real hardware — see `README.md`.

---

## Title
Add a custom URL-scheme handler API (`Options.SchemeHandlers` / `NewWithOptions`)

## Motivation
Desktop apps built on glaze currently serve their frontend assets over a local
loopback HTTP server (`http://127.0.0.1:<port>`), because the browser engine
treats loopback as a **secure context** — required for `localStorage`,
`crypto.subtle`, `getUserMedia`, and history-based routing.

To drop that TCP port entirely, the app needs to serve embedded assets from a
custom scheme (e.g. `app://`) that the engine **also treats as a secure
context**. Every backend glaze wraps supports this, but glaze exposes no hook:

- **macOS (WKWebView):** `-[WKWebViewConfiguration setURLSchemeHandler:forURLScheme:]`
  — must be set **before** the `WKWebView` is created (the configuration is copied
  at init), so it cannot be added from outside glaze. This is the key reason the
  feature must live in glaze.
- **Linux (WebKitGTK):** `webkit_web_context_register_uri_scheme` +
  `webkit_security_manager_register_uri_scheme_as_secure`.
- **Windows (WebView2):** `ICoreWebView2.AddWebResourceRequestedFilter` +
  `WebResourceRequested`, over an `https://` virtual host for a secure context.

Verified: a custom scheme registered this way reports `window.isSecureContext ===
true` with working `localStorage` + `crypto.subtle` on **real WKWebView (macOS 14)**
and **WebKitGTK (GTK3 + GTK4)**.

## Proposed API
Additive and backward-compatible — existing `New`/`NewWindow` keep working:

```go
type SchemeRequest struct { URL, Method string }
type SchemeResponse struct {
    Body       []byte
    MIMEType   string            // default application/octet-stream
    StatusCode int               // default 200
    Headers    map[string]string
}
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

Handlers are install-time only (macOS freezes the config at init), so they are an
Option to `NewWithOptions`, not a method on the running `WebView`.

## Status of a reference implementation
- **macOS** — implemented: a `WKURLSchemeHandler` class set on the config before
  `initWithFrame:configuration:`. ✅ verified on `macos-14`.
- **Linux** — implemented: registers on the view's `WebKitWebContext` and marks
  the scheme secure. ✅ verified on GTK3 + GTK4 under xvfb.
- **Windows** — API present (`NewWithOptions`), scheme wiring is a TODO
  (`AddWebResourceRequestedFilter` + `WebResourceRequested`). Happy to complete it
  if the API shape is acceptable.

Would you accept a PR along these lines? Any preference on the API shape (e.g. a
`RegisterScheme` builder vs. the `Options` map, response modeling)?

---

**Fork + upstream note (for goleo, not for the issue):** goleo ships this via a
pinned fork regardless (pre-1.0, single-maintainer insulation —
`scripts/pin-glaze-fork.*`), so upstream merging is cleanup, not a blocker. goleo
already consumes glaze's scheme API on **both macOS and Linux** (via
`NewWithOptions` in `runtime/webview_glaze.go`, `Config.SchemeAssets`), verified
end-to-end on `macos-14` + Linux GTK3/GTK4. **Windows** currently falls back to the
loopback server (goleo still wraps `jchv/go-webview2` there), but is being migrated
onto glaze — at which point completing glaze's Windows scheme wiring becomes
directly useful to goleo, not out-of-scope.
