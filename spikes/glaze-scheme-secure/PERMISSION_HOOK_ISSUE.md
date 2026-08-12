# glaze issue draft — permission-request hook

**Internal (do not post):** Per glaze's CONTRIBUTING, a new API needs an issue
first to agree scope + the two rules before a PR (the scheme API took this
route). Post only the block between the `POST FROM HERE` / `END POST` markers.
This is deliberately a *host-decided hook*, not the auto-grant policy goleo
carries in its fork — glaze stays unopinionated; the app decides.

---
<!-- ===================== POST FROM HERE ===================== -->

**Title:** Add a permission-request hook (`Options.OnPermissionRequest`)

### Problem

A page loaded in the web view can request a capability — `getUserMedia`
(camera/microphone), geolocation — and the engine raises a permission request.
glaze never surfaces it, so nothing answers, and the call **hangs**: on
WebView2 the request sits unhandled; on WebKitGTK the `permission-request`
signal is unconnected. There is no way today for the host to say allow or deny.

This is the web view's own event surface (the request comes *from* the loaded
content), not a window-independent OS binding — the same category as the file
dialogs glaze already exposes, not the clipboard/notification bindings that live
in `native`. A web view that can't answer a permission prompt is incomplete.

### What this is — and isn't

A **hook, not a policy.** glaze does not decide to grant anything; it hands the
request to a host callback that returns the decision. The default (no callback)
is unchanged — glaze keeps deferring to the engine, so this is invisible to
everyone who doesn't opt in. Auto-granting (which suits an app serving only its
own trusted content) becomes a one-line host callback, and glaze stays
unopinionated about a security-sensitive choice.

### Why it fits glaze

- **No CGo (rule 1).** Implemented with **purego / COM / the Obj-C runtime
  only** — `add_PermissionRequested` (WebView2), the `permission-request` signal
  (WebKitGTK), a `WKUIDelegate` method (WKWebView). No `import "C"`, no new
  dependency (purego stays the only one), `CGO_ENABLED=0` green on every target.
- **YAGNI (rule 2).** One optional callback and a small kind enum — no
  speculative knobs, one real use. A nil callback changes nothing, so
  `New`/`NewWindow`/`NewWithOptions` behavior is untouched. It is *not* minimizing
  away security: surfacing the decision to the host is the safe minimum (a blanket
  grant baked into the library would not be).
- **In scope.** The web view binding answering its own permission events —
  core to hosting a web view, like the existing file-dialog support.

### Proposed API

Additive and backward-compatible:

```go
type PermissionKind int

const (
    PermissionCamera PermissionKind = iota
    PermissionMicrophone
    PermissionGeolocation
)

type PermissionRequest struct {
    Kind   PermissionKind
    Origin string // requesting origin, e.g. "https://app.localhost"
}

type Options struct {
    // ... existing fields ...

    // OnPermissionRequest decides camera/microphone/geolocation requests from
    // loaded content. Returning true allows, false denies. A nil callback keeps
    // the engine's default behavior (glaze does not intercept). Runs on the UI
    // thread; keep it quick.
    OnPermissionRequest func(PermissionRequest) bool
}
```

### Per-backend mechanism

- **Windows (WebView2):** `ICoreWebView2.add_PermissionRequested`; read
  `PermissionKind` + `Uri` off the event args, set `State` to
  `ALLOW`/`DENY` from the callback (or leave `DEFAULT` when no callback).
- **Linux (WebKitGTK):** connect the view's `permission-request` signal; branch
  on the concrete `WebKitPermissionRequest` (user-media / geolocation) and call
  `webkit_permission_request_allow()` / `_deny()`.
- **macOS (WKWebView):** camera/microphone via the `WKUIDelegate`
  `webView:requestMediaCapturePermissionForOrigin:initiatedByFrame:type:decisionHandler:`
  (macOS 12+), invoking the decision handler with grant/deny.

### One honest platform gap

macOS **geolocation** does not come through `WKUIDelegate`; WKWebView defers to
CoreLocation system authorization, so it can't be answered by this hook. Per the
"all three, or honest about it" rule I'd document geolocation as
macOS-unsupported (camera/microphone work on all three; geolocation on
Windows + Linux). "Works on two, honest about the third" rather than pretending.

### Questions before a PR

1. Do you want permission handling in glaze at all, or is it a host concern to
   keep out?
2. If yes: the decision shape — a `bool` (allow/deny) as above, or a tri-state
   (`Allow`/`Deny`/`Default`) so a host can allow some kinds and let the engine
   prompt for others? I lean `bool` (YAGNI) but will follow your preference.
3. The kind set — start at camera/microphone/geolocation, or include more
   (notifications, clipboard) from the start?

Happy to implement all three backends with the macOS-geolocation caveat and a
headless test for the decision mapping, matching the scheme PR's shape.

<!-- ===================== END POST ===================== -->
---

**Internal — goleo consumption (do not post):** goleo currently carries this as
an unconditional auto-grant in the fork's WebView2 backend
(`daforester/glaze`, commit `953debd`). If this hook lands upstream, goleo drops
that fork commit and instead registers an auto-allow callback in its own runtime
(`OnPermissionRequest: func(PermissionRequest) bool { return true }`), the policy
living in the app where it belongs. Combined with an upstream release that
carries the scheme API, this is the last thing keeping goleo on the fork — see
`SPIKES.md`.
