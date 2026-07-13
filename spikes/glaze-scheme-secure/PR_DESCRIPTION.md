# Add custom URL-scheme handlers (`NewWithOptions` / `Options.SchemeHandlers`)

*Serve a web view's assets from your own `app://`-style origin — one that WebKit
and WebView2 treat as a **secure context** — without opening a TCP port.*

This is an **additive, backward-compatible** change: `New` and `NewWindow` keep
working exactly as before. It's implemented in the same **cgo-free purego** style
as the rest of glaze and adds **no new dependencies**.

---

## The problem it solves

Today, a glaze-based desktop app that wants to load a real frontend (a Vite/React/
Vue bundle, client-side routing, `localStorage`, `crypto.subtle`, camera access…)
has two options, and both have a sharp edge:

| Approach | Port? | Secure context? | Reality |
|----------|-------|-----------------|---------|
| Run a local `http://127.0.0.1:<port>` server and `Navigate` to it | **Opens a TCP port** | ✅ yes | Works, but every window now depends on a listening socket — a port to bind (and collide on), and a surface other local processes can reach. |
| `SetHtml` / `file://` | No port | ❌ **no** | `window.isSecureContext === false` → `crypto.subtle` is undefined, `getUserMedia`/geolocation are blocked, `localStorage` is unreliable, and you're stuck with hash-only routing / an opaque origin. |

There's no way today to get the **best of both**: no port *and* a secure context.
That "secure context" isn't a nice-to-have — a large slice of the modern web
platform (`crypto.subtle`, `localStorage` guarantees, `getUserMedia`, service
workers, and more) is gated behind it.

Every mature webview toolkit (Tauri, Wails, Electron) solves this the same way:
a **custom scheme registered as secure**, serving embedded assets in-process.
glaze already wraps the three native engines that can do this — it just doesn't
expose the hook. This PR adds it.

---

## What this PR adds

A small, focused API. You hand `NewWithOptions` a map of scheme → handler; the
handler turns a request into bytes:

```go
wv, _ := glaze.NewWithOptions(glaze.Options{
    SchemeHandlers: map[string]glaze.SchemeHandler{
        "app": func(req *glaze.SchemeRequest) *glaze.SchemeResponse {
            data, ctype := assets.Serve(req.URL) // your embedded FS, however you like
            return &glaze.SchemeResponse{Body: data, MIMEType: ctype}
        },
    },
})
wv.Navigate("app://home/index.html") // secure origin, no port
```

New public surface (one new file, `scheme.go`):

```go
type SchemeRequest  struct { URL, Method string }
type SchemeResponse struct { Body []byte; MIMEType string; StatusCode int; Headers map[string]string }
type SchemeHandler  func(*SchemeRequest) *SchemeResponse
type Options        struct { Debug bool; Window unsafe.Pointer; SchemeHandlers map[string]SchemeHandler }

func NewWithOptions(opts Options) (WebView, error)
```

Handlers are supplied at construction (not added later) because macOS bakes the
scheme handlers into the `WKWebViewConfiguration` **before** the `WKWebView`
exists — so the option belongs on the constructor. `New` / `NewWindow` now simply
delegate to `NewWithOptions`, so nothing existing changes.

---

## How it works, per backend

Each engine has a first-class, documented mechanism for exactly this — the PR just
wires glaze's `SchemeHandler` to it, all through the existing purego bindings:

- **macOS (WKWebView):** registers a `WKURLSchemeHandler` on the configuration
  before the view is created; responses are returned via the URL-scheme task.
- **Linux (WebKitGTK):** `webkit_web_context_register_uri_scheme` on the view's
  context, marked secure with
  `webkit_security_manager_register_uri_scheme_as_secure`; the body is streamed
  from an in-memory `GInputStream` (`g_memdup2` + `g_free` destroy-notify, so no
  leaks and no lifetime foot-guns).
- **Windows (WebView2):** `NewWithOptions` is added for API parity; the scheme
  wiring itself (`AddWebResourceRequestedFilter` + `WebResourceRequested` over an
  `https://` virtual host) is left as a clearly-marked TODO. Happy to complete it
  in this PR or a follow-up — flagged honestly rather than shipped half-done.

---

## Why it's worth merging

- **Unlocks portless, single-binary desktop apps.** Embed your frontend, serve it
  straight from Go, open **zero** sockets. No port to bind, no port collisions, no
  loopback surface for other local processes — while the page still behaves like a
  first-class web origin.
- **Keeps the secure context.** `localStorage`, `crypto.subtle`, `getUserMedia`,
  service workers, and real path-based routing all keep working — the exact things
  `file://` and `SetHtml` quietly break.
- **Brings glaze to feature parity** with Tauri/Wails/Electron on custom
  protocols — a capability real apps ask for, widening glaze's addressable use
  cases, with none of their cgo/Rust/Chromium baggage.
- **Zero risk to existing users.** Purely additive; `New`/`NewWindow` are
  untouched in behavior (they delegate). If you never pass `SchemeHandlers`,
  nothing changes.
- **Stays true to glaze's design.** cgo-free (purego only), no new dependencies,
  one new file plus small, self-contained per-backend additions.

---

## Verified on real hardware, not just compiled

A probe page served from a custom scheme reported **`window.isSecureContext ===
true` with working `localStorage` and `crypto.subtle`** on:

- **macOS 14 (Apple Silicon)** — real WKWebView, via GitHub Actions.
- **Linux** — real WebKitGTK on **both GTK3 (webkit2gtk-4.1)** and **GTK4
  (webkitgtk-6.0)**, under xvfb.

All builds are `CGO_ENABLED=0` and cross-compile for darwin/{amd64,arm64},
linux/{amd64,arm64}, windows/amd64.

---

## Scope & compatibility

- **Additive and backward-compatible** — no existing signature or behavior changes.
- **Windows scheme wiring is a TODO** (the API is present); everything else is
  implemented and verified.
- No new module dependencies; no cgo.

Glad to adjust the API shape to your preference (e.g. a builder method vs. the
`Options` map, or the response model) — this is meant to fit glaze's conventions,
not impose new ones.
