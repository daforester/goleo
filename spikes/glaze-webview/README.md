# Spike — `crgimenes/glaze`: cgo-free macOS/Linux webview (2026-07-12)

**Question:** is there an importable, cgo-free (`CGO_ENABLED=0`) Go webview binding
for **macOS (WKWebView)** and **Linux (WebKitGTK)** — so Goleo can drop the cgo
`webview/webview_go` backend and flip `darwin`/`linux` to pure Go, the way
`jchv/go-webview2` already does for Windows?

**Answer: YES — [`github.com/crgimenes/glaze`](https://github.com/crgimenes/glaze).**
It is essentially the productized form of what Goleo's Spikes 1 & 2 built by hand:
a purego/`dlopen` reimplementation of WKWebView, WebKitGTK, *and* WebView2 behind
one `WebView` interface, on the same `ebitengine/purego` stack the spikes used.

## Verified here (from a Windows host, no Mac/Linux needed)

`go build` of `main.go` + `wrapper_reference.go` (which exercise the full API
Goleo needs), `CGO_ENABLED=0`:

| Target | Builds | `runtime/cgo` in deps |
|---|---|---|
| darwin/amd64 | ✅ | 0 |
| darwin/arm64 | ✅ | 0 |
| linux/amd64 | ✅ | 0 |
| linux/arm64 | ✅ | 0 |
| windows/amd64 | ✅ | 0 |

Zero `import "C"` anywhere in glaze. So it is genuinely cgo-free and
**cross-compiles from one machine** — the core thesis, confirmed. (Interactive
GUI/UX is *not* proven here; that still needs real hardware.)

## Why adoption is cheap

glaze's `WebView` interface is the same
`New/Navigate/SetTitle/SetSize/Eval/Init/Bind/Run/Destroy/Dispatch/Terminate`
shape Goleo already wraps in `runtime/webview_windows.go`, so the mac/Linux
backend is a ~1:1 wrapper — see [`wrapper_reference.go`](wrapper_reference.go),
which compiles against glaze and includes a compile-time assertion that
`glaze.WebView` satisfies `runtime/nativeipc.go`'s `nativeEvaler` push interface
(so native IPC needs **no** per-backend change).

glaze also already solves the two things `SPIKES.md` flagged as remaining Linux
work: **GTK3/GTK4 mutual exclusion** and **WebKitGTK version fragmentation**
(4.0/libsoup2 · 4.1/libsoup3 · 6.0/GTK4) — it runtime-detects and loads exactly
one stack per process.

## Adoption plan (Phase 1 — flip darwin/linux to pure Go)

See the migration checklist at the top of `wrapper_reference.go`. In short:
add `runtime/webview_darwin.go` + `runtime/webview_linux.go` (the wrapper),
delete the cgo `runtime/webview.go`, drop `webview/webview_go`, and remove the
`else CGO_ENABLED=1` branch in `cli/cmd/build.go`.

## Caveats before depending on it

- **Young / pre-1.0 / single-maintainer** (v0.0.31, ~122★). **Vendor or fork and
  pin a commit** — trade "write the binding" for "co-own a small dependency".
- **Real-hardware verification required:** run Goleo's native-IPC `{type,data}`
  round-trip through glaze's `Bind` against `Bridge.HandleRequest` (the Spike 2
  test) on real macOS (GH Actions) + Linux (xvfb/box) before trusting it.
- glaze's **Linux native menu bar is `ErrUnsupported`**; check its asset-serving
  against Goleo's loopback/token model.
- macOS multi-window still needs the single-loop master (AppKit is
  main-thread-only) — glaze gives the binding, not that architecture.

## Run it

```
cd spikes/glaze-webview
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o /dev/null .
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o /dev/null .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /dev/null .
```
