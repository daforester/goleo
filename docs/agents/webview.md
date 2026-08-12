# Desktop webview (glaze) and window modes

> Split out of `AGENTS.md`, which loads into every session. **Read it before changing anything under runtime/webview*.go, the glaze dependency or fork, window modes, or multi-window behaviour.**
>
> `AGENTS.md` carries the invariants that must be known even without reading this;
> everything here is the detail behind them. `SPIKES.md` has the evidence and the
> hardware-verification history.

---
## WebView / Native Window

Goleo renders the desktop frontend in a **native OS webview**. As of the glaze
unification, **all three desktops use ONE cgo-free binding by default**:
- **Default (all desktops): `github.com/crgimenes/glaze`** (`runtime/webview_glaze.go`,
  pinned to the `daforester/glaze` fork) — a **cgo-free** purego binding to
  **WKWebView (macOS)**, **WebKitGTK (Linux)** and **WebView2 (Windows)** behind one
  interface. So every desktop builds `CGO_ENABLED=0` and cross-compiles from any host,
  and goleo carries a single webview binding. Permission auto-grant
  (camera/mic/geolocation): a purego `permission-request` shim on Linux
  (`runtime/webview_glaze_permissions_linux.go`); a `PermissionRequested`→Allow COM
  handler in the glaze fork's WebView2 backend on **Windows** (getUserMedia would
  otherwise hang on an unanswered prompt); no-op on macOS. Verified on real macOS +
  Linux (`.github/workflows/glaze-verify.yml`) and Windows (local: native IPC, scheme
  assets, in-process multi-window, tray, native menu bar, permission grant, clean Quit).
- **No webview fallback:** glaze is the only desktop webview backend. The legacy cgo
  `webview_go` backend and the Windows `go-webview2` backend have both been removed, so
  there is no cgo webview path left. Two pieces of `cgo`-tagged code remain, both Linux
  and both with a pure-Go fallback, so a `CGO_ENABLED=0` build is unaffected:
  `runtime/camera`'s V4L2 impl (`camera_linux.go`, stubbed elsewhere — camera then routes
  to the WebView's `getUserMedia`) and `runtime/nfc`'s libnfc impl
  (`nfc_libnfc_linux.go`), which is additionally opt-in behind `-tags goleo_libnfc`.

So **every desktop target is pure-Go and cross-compilable from one machine**, on a
single binding. Shutdown unblocks the run loop via `endRunLoop()`
(glaze's `Terminate()`) — not a GOOS check.

### Why goleo pins a glaze fork (`daforester/glaze`, currently `v0.0.46-goleo.1`)

goleo `replace`s `crgimenes/glaze` with the fork for **exactly one reason: the
Windows WebView2 permission auto-grant.** The custom-scheme API the fork was
originally created for is **merged into upstream glaze and released** (it ships in
`v0.0.46`), so the fork no longer carries it — the fork is now a **rebase of
upstream `v0.0.46` plus one commit**, and its whole delta is the `go.mod` module
line plus the ~50-line grant (`webview2_permissions_windows.go` + its wiring). The
`PermissionRequested`→Allow handler was deliberately **kept out of the upstream
PR**: auto-granting camera/mic/geolocation is a security *policy* that suits goleo
(it loads only its own trusted content) but is wrong as a default for a general
library.

Note the grant is **not** an API goleo calls — it is internal to glaze. So a
downstream module that drops the `replace` still *compiles*; it just silently loses
camera/mic/geolocation on Windows. That is why `goleo new` scaffolds the `replace`
into the generated `go.mod` (Go replace directives don't transit).

**Why that grant has to live in the fork** (unlike Linux, which goleo handles in
its own runtime): glaze owns the WebView2 setup and exposes only the **HWND** via
`Window()`, not the `ICoreWebView2` COM interface — and WebView2 has no
HWND→interface recovery. `PermissionRequested` is a COM event *on* `ICoreWebView2`,
so there is no external handle to attach it to; it must be wired inside glaze. By
contrast, on **Linux** the equivalent is a GObject signal on the `WebKitWebView`
(which `Window()` *does* reach), so goleo attaches it **externally**
(`runtime/webview_glaze_permissions_linux.go`) with no fork. On **macOS** WKWebView
grants media itself — no-op (`runtime/webview_glaze_permissions_other.go`).

**Path off the fork (now the only remaining step):** land an upstream
permission-request *hook* (glaze surfaces the request, the host returns allow/deny;
goleo supplies an auto-allow callback) — drafted in
`spikes/glaze-scheme-secure/PERMISSION_HOOK_ISSUE.md`. The scheme-API half of this
condition is already satisfied, so once that hook exists upstream the `replace`
goes away entirely and goleo pins plain `crgimenes/glaze`. Full history in
`SPIKES.md`.

**Rebasing the fork onto a newer upstream** (`scripts/pin-glaze-fork.*` pins it;
the rebase itself is manual): branch off the new upstream tag, cherry-pick the
grant commit, rewrite `go.mod`'s module line to `github.com/daforester/glaze`, tag
`<upstream>-goleo.N`, then re-pin + `go mod vendor`. Leave every other upstream
file byte-identical — in particular do **not** repoint upstream's
`editor_scenario_darwin_test.go` import at the fork: `go mod tidy` in a *consuming*
module walks a dependency's test imports, and that would make it resolve
`daforester/glaze` as an external module instead of through the consumer's
`replace`, breaking `go mod tidy` for every downstream project.

### Window modes (`Config.WindowMode`)

- `WindowModeWebview` — native OS webview window. This is the **default for
  scaffolded desktop builds** (the generated `main.go` sets it). `App.Run()`
  calls `runWebview()`, which either reuses the window created by `init.js`'s
  `createWindow()` or opens one pointed at the embedded server (prod) / Vite
  dev URL.
- `WindowModeBrowser` — no native window; the app serves its UI and you open it
  in a browser. Used for PWA builds and `goleo emulate`/dev tooling. In this
  mode `createWindow()` in `init.js` is a no-op.
- `WindowModeMobile` — mobile hosting.

The webview auto-grants OS permission prompts (camera/mic/geolocation) so the
frontend's browser-API fallbacks resolve instead of hanging: Linux via a purego
`permission-request` shim (`webview_glaze_permissions_linux.go`), Windows via the
fork's WebView2 `PermissionRequested` handler, and a no-op on macOS
(`webview_glaze_permissions_other.go`). See "Why goleo pins a glaze fork" above for
why the Windows grant lives in the fork.

On mobile, the native Android/iOS shell hosts the platform WebView (Android
WebView / WKWebView) and loads the Go server, so mobile entry points use
`WindowModeBrowser`; the desktop webview is compiled out under the
`mobilebuild` build tag (`webview_stub.go`).

Window creation can also be scripted from `init.js` through the embedded JS
engine (`createWindow`/`getConfig`); see `runtime/jsruntime.go`.

### Multi-window (desktop)

Native OS webviews are single-window and own the GUI thread, so **additional
windows run as child processes** of the same binary — each hosts one webview
pointed at the shared backend server, reusing the existing WebSocket hub for
cross-window IPC. The main process stays the sole backend/controller; the
primary window is still hosted in-process by `runWebview`.

- `runtime/windowmanager.go` — `WindowManager` spawns/tracks/kills child window
  processes; `App.OpenWindow(WindowOptions)` is the Go entry point.
- `runtime/window_child.go` — a process launched with `GOLEO_WINDOW=1` (+ URL/
  title/size env vars) is detected at the top of `App.Run` and hosts one webview
  instead of starting a server.
- Bridge commands `goleo:windowOpen` / `goleo:windowClose` / `goleo:windowList`
  (registered in `App.registerWindowCommands`) drive it from the frontend;
  `bridge/src/window.ts` wraps them (`openWindow`/`closeWindow`/`listWindows`).
  Events `window:opened` / `window:closed` are emitted on the bridge.

This is cgo-free and binding-agnostic (works with either webview backend).
