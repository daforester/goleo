# Spike — `goleo://` custom-scheme **secure-context** verification

**Question this gates:** is a uniform, all-desktop-platforms `goleo://` custom
scheme (to drop the last loopback TCP port a desktop app opens) actually possible?

Serving embedded assets over a custom scheme is not the hard part. The hard part
is whether the custom origin is a **secure context** — the property
`http://127.0.0.1` gives us today, and the whole reason to bother. Without it,
`localStorage`, `crypto.subtle`, `getUserMedia`, and history routing break.

The three backends are **not equal** on this:

| Backend | Mechanism | "Mark as secure" API? | Needs glaze fork? |
|---------|-----------|-----------------------|-------------------|
| **Windows** (WebView2) | `SetVirtualHostNameToFolderMapping` over `https://` | https ⇒ secure implicitly | No — `go-webview2` already exposes it |
| **Linux** (WebKitGTK) | `webkit_web_context_register_uri_scheme` + handler | `webkit_security_manager_register_uri_scheme_as_secure` (explicit) | No — attachable via purego, like the permission shim |
| **macOS** (WKWebView) | `WKURLSchemeHandler` set on the config before init | **none public** — the gating unknown | Yes (config is frozen at init) — but only if it even reports secure |

So **macOS decides whether the uniform PR is possible at all.** This spike loads
the *same* probe page (`probe.go`) from the custom origin on each backend; the
page reports `isSecureContext` + a real `localStorage` write + a real
`crypto.subtle.digest` back to Go. PASS on a platform == viable there.

## Files
- `probe.go` — shared probe HTML + report parsing + PASS/FAIL logic (no build tag).
- `main_windows.go` — `edge.Chromium` + vhost mapping (`https://goleo.assets/`). cgo-free.
- `main_linux.go` — glaze + an **external purego shim** that registers `goleoapp://`
  as a secure scheme on the web view's `WebKitWebContext`. **No glaze change.**
- `main_darwin.go` — raw purego/objc `WKWebView` + `WKURLSchemeHandler` for
  `goleoapp://`. Simulates exactly what a glaze fork would do, to answer the
  secure-context question on real WKWebView.

## Results (2026-07-13)

| Platform | How verified | Result |
|----------|--------------|--------|
| **Windows/WebView2** | local, real hardware (dev desktop) | ✅ **PASS** — `https://goleo.assets` secure; localStorage + WebCrypto work |
| **Linux/WebKitGTK (GTK3, webkit2gtk-4.1)** | Docker + xvfb (`goleo-linux-verify`) | ✅ **PASS** — `goleoapp://app` secure |
| **Linux/WebKitGTK (GTK4, webkitgtk-6.0)** | Docker + xvfb + dbus (`goleo-linux-verify-gtk4`) | ✅ **PASS** — `goleoapp://app` secure |
| **macOS/WKWebView** | cross-compiles `CGO_ENABLED=0`; **awaiting `macos-14` runner** | ⏳ pending (the gating result) |

## Run it

```sh
# Windows (native):
cd spikes/glaze-scheme-secure && CGO_ENABLED=0 go build -o scheme-spike.exe . && ./scheme-spike.exe

# Linux (Docker, from repo root) — GTK3 then GTK4:
docker run --rm -v "$PWD/spikes/glaze-scheme-secure:/work" goleo-linux-verify \
  bash -c "cd /work && CGO_ENABLED=0 go build -o /tmp/bin . && xvfb-run -a /tmp/bin"

# macOS: runs on the macos-14 runner via .github/workflows/glaze-verify.yml.
```

## What a PASS everywhere means

The all-platforms `goleo://` PR is viable and the shape is:
- **Windows:** goleo's own Windows wrapper reaches `edge.Chromium` (already a dep) — no fork.
- **Linux:** an external purego shim in goleo's runtime (like the permission shim) — no fork.
- **macOS:** a **small glaze change** (set `WKURLSchemeHandler` on the config before
  `initWithFrame:configuration:`, exposed through glaze's API) — the only fork needed,
  ideally upstreamed. Fork tooling already exists (`scripts/pin-glaze-fork.*`).

A macOS **FAIL** (custom scheme not a secure context) means the uniform PR is not
possible today; the loopback asset server stays (a small residual, since native
IPC already removed the RPC/WS surface).
