# SPIKES.md — Feasibility findings

Durable record of the de-risking spikes run for the desktop / cgo-free / in-process
architecture. These results are the evidence behind the decisions in
[`docs/roadmap.md`](docs/roadmap.md). **Don't re-run these from scratch — read here first.**

Dates are when verified. Environment: Windows 11 host, Go 1.26, Docker (Linux), GitHub Actions
(macOS), an Android emulator.

> **History was flattened on 2026-08-12 (at v0.11.0).** `master` is a single root commit, so
> commit SHAs cited in entries below **do not resolve** unless a release tag still contains
> them — `8a600d2` and `1983378` survive via `v0.10.12`/`v0.10.13`, but anything committed
> after `v0.10.13` and before the flatten does not exist as an addressable commit any more.
> The pre-flatten history is preserved outside the repo in
> `goleo-pre-flatten-2026-08-12.bundle`. Prefer describing *what changed* over citing a SHA in
> new entries; this is the second flatten (the first was 2026-08-06) and it will happen again.

---

## TL;DR — the cgo-free thesis holds on all three desktop OSes

A native OS webview inherently binds C/ObjC APIs, so historically it needs cgo. The spikes
proved a **cgo-free** path on every desktop OS:

| OS | Mechanism | Status | Verified how |
|----|-----------|--------|--------------|
| **Windows** | `github.com/jchv/go-webview2` (WebView2 via COM/syscall) | ✅ builds + runs | `CGO_ENABLED=0` build; ran multi-window on the dev's desktop |
| **Linux** | `purego` + `dlopen` of GTK/WebKit | ✅ mechanism proven | `dlopen`+call in a `golang:1.26` container (Spike 1) |
| **macOS** | `purego` + WKWebView | ✅ JS↔Go on real hardware | GitHub Actions Apple-Silicon runner (Spike 2) |

Consequence: builds stay `CGO_ENABLED=0` and **cross-compilation works** (darwin was
cross-built from Windows). Per-OS runners are still needed for signing/notarization/packaging
and interactive GUI testing — not for compilation.

**Current binding (update):** Windows later **migrated off `go-webview2` onto glaze**
(2026-07-14 — see the "Windows → glaze migration" section below), so all three desktops now
share **one** cgo-free binding, `crgimenes/glaze` via purego. The `go-webview2` row above is the
*original* Windows proof; several mid-document 2026-07-13 findings that treat `go-webview2` as the
Windows backend (and "Windows SchemeAssets falls back to loopback") are dated history superseded by
that migration. **And (2026-07-20) the legacy cgo `webview_go` fallback was removed entirely** (dep,
`runtime/webview.go`, the `goleo_cgo_webview` tag, and the webkit pkg-config shim all gone), so glaze
is now the *sole* desktop webview and there is no cgo webview path — the only `cgo`-tagged code left
is `runtime/camera`'s Linux V4L2 impl (with a pure-Go stub). [Correction, 2026-08-04: there are
**two**, not one — `runtime/nfc`'s libnfc impl (`nfc_libnfc_linux.go`) is also cgo, though it is
additionally opt-in behind `-tags goleo_libnfc`, so a default `CGO_ENABLED=0` build is still
unaffected. The conclusion stands; the count was wrong.] Later mentions below of `webview_go`
being "retained one release, then removable" are that closed-out plan.

---

## Spike 0 — the CGO_ENABLED=0 vs `webview_go` defect (2026-07-09)

**Finding:** `CGO_ENABLED=0 go build ./runtime/...` fails — `"build constraints exclude all Go
files in …/webview_go"` — because `github.com/webview/webview_go` is entirely cgo-gated.
`goleo build` forced `CGO_ENABLED=0`, so the native-webview desktop path could not compile;
it only worked under a cgo build (`go run`).

**Decision:** native webview needs cgo *or* a cgo-free binding. → adopt go-webview2 (Windows,
cgo-free) + purego (mac/linux); set per-OS `CGO_ENABLED` in `buildForDesktop` (Windows 0).

---

## Spike: go-webview2 is cgo-free on Windows (2026-07-09)

**Method:** scratch module, `go get github.com/jchv/go-webview2`, build a WebView2 app with
`CGO_ENABLED=0 GOOS=windows`.

**Result:** ✅ builds (3.9 MB exe); `go list -deps` shows **no `runtime/cgo`** in the tree
(COM via `syscall` + `go-winloader`). Public API mirrors `webview_go`
(`New/Navigate/SetTitle/SetSize/Eval/Run/Destroy`), plus a lower-level `pkg/edge` layer
(`Chromium`, `WebResourceRequested`, `CreateWebResourceResponse`, `Bind`) usable for custom
schemes / multi-window.

---

## Spike 1 — Linux cgo-free `dlopen` via purego (2026-07-09) ✅ PASS

**The feared blocker:** a `CGO_ENABLED=0` Go binary is normally statically linked on Linux and
cannot `dlopen`.

**Method:** `golang:1.26` Docker container; `purego.Dlopen("libgtk-3.so.0")` +
`RegisterLibFunc` → call `gtk_get_major_version()`, across three build modes.

**Result:** returned `3` under **`CGO_ENABLED=0` (default build)**, `CGO_ENABLED=0
-buildmode=pie`, and `CGO_ENABLED=1` — all exit 0. purego's `//go:cgo_import_dynamic`
directives make even the `CGO_ENABLED=0` binary **dynamically linked** (ELF interpreter
`ld-linux`), so `dlopen` works. The static-binary fear did **not** materialize.

**Caveats / remaining Linux work:** tested amd64 + glibc/Debian with **GTK3** (the `dlopen`
mechanism is version-agnostic). Still to confirm: **arm64**, and a real **`webkitgtk`** lib.
Remaining engineering (not feasibility): GObject signal marshaling (`g_signal_connect_data` +
`purego.NewCallback`), webkit version fragmentation (4.0/libsoup2 · 4.1/libsoup3 · 6.0/GTK4),
main-thread dispatch (`g_idle_add`). Needs the binary dynamically linked + `ld.so` + libs
present at runtime (always true on desktop Linux; a fully-static/musl/distroless target would
not work).

---

## Spike 2 — macOS purego WKWebView JS↔Go (2026-07-10) ✅ PASS on real hardware

**Method:** GitHub Actions `macos-14` (Apple Silicon/arm64), `go1.26.4 darwin/arm64`,
`CGO_ENABLED=0`. A purego/objc spike that:
1. `objc.RegisterClass` a `WKScriptMessageHandler` delegate whose method is a **Go func**
   (`objc.MethodDef{Fn: …}`),
2. loads HTML that calls `window.webkit.messageHandlers.external.postMessage(...)` (JS→Go),
3. from Go calls `evaluateJavaScript` to post back (Go→JS), completing a round-trip.

**Result:** `RESULT: PASS`. The delegate fired both times. Two behaviors that were unverified
beforehand **worked first try**: passing a **`CGRect` struct-by-value** to
`initWithFrame:configuration:`, and a **nil `completionHandler`** to `evaluateJavaScript:`.
No cgo, no local Mac. Also: the same spike **cross-compiled from Windows** for darwin/arm64 +
darwin/amd64 (`CGO_ENABLED=0`).

**purego/objc API used:** `Dlopen`/`Dlsym`/`RegisterLibFunc`; `objc.GetClass`, `RegisterName`,
`ID.Send`, generic `Send[T]`, `RegisterClass(name, super, protocols, ivars, []MethodDef)`,
`Class.AddMethod`, `NewIMP`, `MethodDef{Cmd SEL, Fn any}`. Production-proven on macOS by
Ebitengine.

**Caveats:** ran headless — interactive window/dock UX and the `WKURLSchemeHandler` asset path
are **unexercised**; the `macos-13`/amd64 matrix job was not confirmed; gomobile's Swift
arg-label generation for multi-arg methods is a guess (iOS provider wiring is unverified).

---

## Spike — gogpu/systray is cgo-free (tray) (2026-07-09)

**Method:** `go get github.com/gogpu/systray@v0.1.1`; build a tray app `CGO_ENABLED=0
GOOS=windows`.

**Result:** ✅ builds (no `runtime/cgo`; uses `go-webgpu/goffi` FFI + `godbus` on Linux). API:
`New()`, `SetIcon/SetTooltip/SetMenu`, `OnClick/OnRightClick`, `ShowNotification`, `Run()`,
`Remove()`; `NewMenu().Add(label, onClick)`.

**Critical constraint:** `internal/init.go` calls `runtime.LockOSThread()` and `tray.Run()`
owns the **main thread's** loop. A native webview also wants the main thread → **a tray app
forces the main process to be a headless controller with windows as child processes** (or an
in-process single-loop that the tray shares). This mandated the "hidden-master" lifecycle
model, not just suggested it.

---

## Spike (D4) — Windows in-process multi-window (2026-07-10) ✅ PASS on the dev's desktop

**Question:** can `go-webview2` host two windows in one process, cheaply, without the
`edge`-layer single-loop rewrite?

**Method:** `spikes/win-multiwindow/` — two `webview2.NewWithOptions` windows, each on its own
`runtime.LockOSThread` goroutine (Windows gives each thread a message queue), with distinct
WebView2 data dirs. `CGO_ENABLED=0 GOOS=windows`.

**Result:** ✅ two independent windows appeared and worked, one process, two UI threads.

**Decision:** in-process multi-window on **Windows** is cheap — no `edge` single-loop rewrite
needed for basic multi-window; each window = one `LockOSThread` goroutine running `Run()`.
This is the D4.0 Windows path (the alternative to the shipped multi-process `WindowManager`).
Cross-thread control (close a window from the backend) uses the webview's `Dispatch(func)` +
`Terminate`. macOS is the exception: AppKit is main-thread-only, so extra windows there still
need the single-loop richer binding (not the per-thread trick).

---

## Spike — native IPC transport + custom-scheme asset serving (2026-07-12)

**Native IPC (`Config.NativeIPC`) — ✅ SHIPPED + verified on real WebView2.** The frontend can
talk to the `Bridge` over the webview's own channel (`Bind` for JS→Go, `Eval(window.__goleoRecv)`
for Go→JS) instead of the loopback WebSocket, using the same `{type,data}` envelope. Verified on
the dev's Windows desktop (cgo-free): a two-window app where each window (primary + an in-process
`InProcessWindows` window) completed an independent bidirectional round-trip over its own
`nativeSession`, incl. `goleo:windowOpen` invoked *over* native IPC, then a clean `Quit`/exit.
`@goleo/bridge` auto-detects the native channel and falls back native → WebSocket → HTTP, so
child-*process* windows, browser/PWA and mobile are unaffected. See `runtime/nativeipc.go`.
- **Two GUI-lifecycle bugs this exposed (both fixed):** (1) `StartServer` overwrote the cancellable
  `a.ctx` that `Run` installed with a fresh `context.Background()`, orphaning `a.cancel()` so
  `Quit` hung — `StartServer` now preserves an existing `a.ctx`. (2) The Go main goroutine isn't
  thread-pinned, but the native webview is thread-affine (window messages + `Dispatch` target the
  creating thread), so cross-thread teardown missed — `Run` now `runtime.LockOSThread()`s.

**Custom-scheme asset serving (`goleo://`) — ⏸ DEFERRED to the purego milestone.** Native IPC
removes the WS/RPC surface, but the primary window still loads its assets over the loopback HTTP
server. Dropping that too needs a native scheme/asset handler per OS. **Finding (why not now):**
- **Windows (`jchv/go-webview2`, cgo-free):** the `pkg/edge` layer *has* the machinery —
  `Chromium.WebResourceRequested`, `AddWebResourceRequestedFilter(filter, ctx)`, `Environment()`,
  and `SetVirtualHostNameToFolderMapping` via `ICoreWebView2_3` — **but** the high-level
  `webview.WebView` we wrap keeps `edge.Chromium` in an unexported `browser` field with no hook.
  Using it means reconstructing the window directly on `edge.Chromium` (own hwnd + message loop +
  WndProc + DPI/permissions) — a ~200-line Win32/COM rewrite, **Windows-only**.
- **macOS/Linux (`webview/webview_go`, cgo):** exposes **no** scheme-handler API at all
  (`WKURLSchemeHandler` / `webkit_web_context_register_uri_scheme` are not surfaced).
- **Decision:** don't fragment the codebase with a Windows-only edge rewrite. The purego mac/Linux
  backends (Spikes 1 & 2) are Goleo's own code, so `goleo://` can be added **uniformly across all
  three OSes** there — WebView2 `WebResourceRequested`/virtual-host mapping, `WKURLSchemeHandler`,
  and `register_uri_scheme` — serving the embedded FS over a virtual (secure-context) origin. Until
  then the loopback asset server stays (127.0.0.1-only, embedded assets, no bridge under native
  IPC — a small residual surface). A cgo-free stopgap exists if ever needed — a single inlined
  bundle via `SetHtml` — but its `about:blank`/opaque origin (no `localStorage`, hash-only routing)
  makes it unsuitable as a default.

---

## Spike — `crgimenes/glaze`: cgo-free mac/Linux webview already exists (2026-07-12) ✅ PASS

**Question:** does the cgo-free macOS (WKWebView) / Linux (WebKitGTK) webview binding that the
purego milestone would otherwise write from scratch **already exist as an importable library** —
the way `go-webview2` handed Windows its cgo-free path?

**Finding: YES — `github.com/crgimenes/glaze`** (v0.0.31, MIT, sole dep `ebitengine/purego`
v0.10.1). A purego/`dlopen` reimplementation of WKWebView, WebKitGTK **and** WebView2 behind one
`WebView` interface — the same `New/Navigate/SetTitle/SetSize/Eval/Init/Bind/Run/Destroy/Dispatch/
Terminate` shape Goleo already wraps in `webview_windows.go`. Built on the exact purego stack
Goleo's Spikes 1 & 2 validated. It even solves the two remaining Linux items this doc flagged:
**GTK3/4 mutual exclusion** and **WebKitGTK version fragmentation** (4.0/4.1/6.0) via runtime
single-stack detection.

**Verified** (`spikes/glaze-webview/`, from a Windows host): a program exercising the full API +
a `WebviewWindow` reference wrapper builds `CGO_ENABLED=0` for darwin/{amd64,arm64},
linux/{amd64,arm64}, windows/amd64; **`runtime/cgo` absent from every dep tree; zero `import "C"`
in glaze** → genuinely cgo-free and cross-compilable from one machine. The wrapper includes a
compile-time assertion that `glaze.WebView` satisfies `runtime/nativeipc.go`'s `nativeEvaler`, so
native IPC needs no per-backend change.

**Live hardware verification (`.github/workflows/glaze-verify.yml`) — ✅ PASS on real macOS +
Linux.** A headed JS↔Go round-trip (`spikes/glaze-webview/verify`, glaze `Bind` + `SetHtml` + a
bound Go func the page calls back into) ran green on **`macos-14` (Apple-Silicon/arm64, real
WKWebView)** and **`ubuntu-latest` (WebKitGTK under xvfb)**, both `CGO_ENABLED=0`. So the cgo-free
backend is proven end-to-end, not just at compile time. `macos-13` (Intel/amd64) was **not** run —
GitHub is retiring Intel macOS runners (the job queues indefinitely); amd64-macOS is the same
purego/objc code path as arm64 and stays compile-guarded in `ci.yml` (darwin/{amd64,arm64} +
linux/{amd64,arm64}).

**Permission auto-grant — shim WRITTEN, NOT yet verified (correction 2026-07-13).** glaze does not
connect WebKitGTK's `permission-request` signal, so a straight default-flip would regress Linux
`getUserMedia`/geolocation. Added a cgo-free purego shim
(`runtime/webview_glaze_permissions_linux.go`) — the pure-Go analog of the cgo
`webview_permissions_linux.go` — that grabs the `WebKitWebView` (the GtkWindow child) and connects
`permission-request` → allow, using `RTLD_NOLOAD` so it never pulls a second GTK major into the
process. **The shim lives in goleo's *runtime*, not in the standalone spike**, so nothing has
actually exercised it yet. An earlier note here claimed it was "verified on real macOS + Linux" via
the spike's `getUserMedia` probe — **that was wrong**: the spike uses RAW glaze (no shim), so its
`getUserMedia` was testing platform/WebKit behavior, not the shim. On the `ubuntu-latest` runner it
happened to return `OverconstrainedError` (a no-camera device-check *before* any prompt, so no grant
was needed); on Debian bookworm's WebKitGTK (local Docker) the same probe **hangs the GTK main loop**
(it prompts, nothing answers) — the exact failure the shim exists to fix. The `getUserMedia` probe
has been removed from the spike (it can't validly test a shim it doesn't include).

**Shim now VERIFIED on Linux via a runtime-level test (2026-07-13).** `spikes/glaze-runtime-verify`
is a real goleo app (glaze default backend, `Config.NativeIPC` + `InProcessWindows`) whose embedded
page calls `getUserMedia` over the secure `http://127.0.0.1` origin. Run under xvfb in the same
Docker image where the *raw* spike hangs, it instead reports `perm ... OverconstrainedError` — i.e.
`getUserMedia` got **past** the permission prompt without hanging → the purego shim fired. Same run
also confirmed **native IPC** (page reached the Bridge over the native channel, `native:true`) and
**`mainLoopWindowManager`** (a 2nd window opened via `App.OpenWindow` on the single loop, both
windows round-tripped), then a clean `Quit`. macOS's shim is a no-op (glaze/WKWebView grants).

**✅ macOS verified on `macos-14` (2026-07-13):** the same `glaze-runtime-verify` app went green on
the Apple-Silicon runner (after fixing the embed fixture + the glaze/systray fakecgo link collision)
— native IPC + in-process 2nd window via `mainLoopWindowManager` + clean `Quit` on real WKWebView.
**So the cgo-free desktop stack is now verified on all three OSes:** Windows (WebView2, cgo-free
build), Linux (WebKitGTK, Docker+CI), macOS (WKWebView, `macos-14`). Remaining macOS caveat: the
system **tray** is excluded there (fakecgo collision) and true pixel-level interactive UX is only
exercised headlessly (the runner has no physical display).

**Local Linux verification via Docker+WSL (2026-07-13):** `scripts/verify-linux-docker.*` +
`scripts/linux-verify.Dockerfile` reproduce the `glaze-verify.yml` ubuntu job locally (golang +
GTK3 + WebKitGTK-4.1 + xvfb; hard `timeout` guard). Both spikes **PASS on real WebKitGTK** this way:
`spikes/glaze-webview/verify` (round-trip) and — importantly — **`spikes/glaze-multiwindow`
(two windows, one run loop, both round-tripped)**, which confirms the single-loop multi-window
mechanism on Linux without CI. This local loop also *found* the getUserMedia hang above (bookworm's
WebKitGTK behaves differently from `ubuntu-latest`), which CI had masked.

**Sequencing decision (2026-07-12):** shim first → re-verify → *then* flip the default. Followed —
though "re-verify" turned out not to have actually exercised the shim (see correction above); the
default flip stands (the cgo backend remains available behind `goleo_cgo_webview`), but the Linux
permission grant is the one piece still needing a runtime-level check.

**Default flipped (2026-07-13): glaze is now the default macOS/Linux backend.** After the
round-trip + permission grant verified on real macOS + Linux, made `webview_glaze.go` the default
(tag `!goleo_cgo_webview`) and `build.go` `CGO_ENABLED=0` for all desktop targets. Verified: every
desktop target (windows + darwin/{amd64,arm64} + linux/{amd64,arm64}) builds `CGO_ENABLED=0` with
no tags, `runtime/cgo`=0 — **all desktops pure-Go, cross-compilable from one machine.** The legacy
cgo `webview_go` backend (`webview.go`) is retained one release behind `-tags goleo_cgo_webview` /
`GOLEO_CGO_WEBVIEW=1`, then removable.

**Impact on the estimate:** Phase 1 (flip darwin/linux to pure Go, single-window) drops from
~2–3 weeks of hand-writing+hardening the FFI/objc/GObject binding to **~1 week** of thin wrappers
+ `build.go` `CGO_ENABLED=0` wiring + dropping `webview_go`. The expensive, risky part is largely
eliminated; real-hardware verification remains the dominant remaining cost.

**Decision / caveats:** adopt by **vendor-or-fork + pin a commit** (pre-1.0, single maintainer —
don't float a tag). Before trusting it, run Goleo's native-IPC `{type,data}` round-trip through
glaze's `Bind` against `Bridge.HandleRequest` (the Spike 2 test) on real macOS + Linux. glaze's
Linux native menu bar is `ErrUnsupported`; its asset-serving must be checked against Goleo's
loopback/token model; macOS multi-window still needs the single-loop master (glaze gives the
binding, not that architecture). Runner-up if we'd rather own the glue: `puregotk` (raw purego
GTK4 + WebKit-6.0 bindings, GTK4-only, experimental). Full write-up: `spikes/glaze-webview/README.md`.

---

## Spike — macOS/Linux in-process multi-window via glaze (2026-07-13)

**Question:** can goleo do in-process multi-window on macOS? The Windows path
(`inProcWindowManager`, one `LockOSThread` goroutine + `Run()` per window) can't port —
**AppKit is main-thread-only**, so a second run loop on another thread is impossible. macOS needs
the *single-loop master*: one `[NSApp run]` on the main thread owning all windows.

**Finding: glaze already supports it.** Its darwin backend shares one `NSApplication`; the second
`NewWindow()` skips the launch handshake (`getAndSetIsFirstInstance()` → false) and just creates
another `NSWindow`, `incWindowCount()`; the app terminates only when the last window closes
(`decWindowCount() <= 0`). Linux (GTK, also main-thread-only) behaves the same. So: create the
primary + `Run()` on the main thread; open extra windows by `Dispatch`-ing `glaze.New()` onto that
thread — **never** call `Run()` on them; close one via its `Destroy()` (decrements the count,
leaves the app running).

**Proof:** `spikes/glaze-multiwindow/` opens window 2 *dynamically* (after the primary loop is
already running, via `Dispatch` once window 1 round-trips) and confirms **both** windows complete
a JS→Go round-trip. Cross-compiles cgo-free (verified from Windows for darwin/{amd64,arm64} +
linux/amd64); runs on `macos-14` + `ubuntu-latest` (xvfb) via `glaze-verify.yml`. **Pending the
hardware run** — this is the macOS-threading behavior that can't be checked headless from Windows.

**goleo integration (next):** a third `windowSpawner` for macOS in-process — `runWebview`
registers the primary window as the main-thread dispatcher; `Open` marshals `NewWebviewWindow`
onto it (channel-synced), `Close` dispatches `win.Destroy()`, and window-count→0 drives the normal
`shutdown()`. Full design in `spikes/glaze-multiwindow/README.md`. macOS in-process multi-window
stays multi-process (the shipped default) until this lands + verifies.

## Feasibility — `goleo://` custom-scheme asset serving (2026-07-13)

**Goal:** drop the loopback HTTP *asset* server (native IPC already removed the RPC/WS surface), so
a desktop app opens no TCP port at all.

**Hard finding: the only portless option that keeps full functionality is a native scheme
registered as a *secure context*, which requires forking glaze.** The cheap alternatives are
inadequate because they lose the secure context that `http://127.0.0.1` currently provides:

| Approach | cgo-free | No port | Secure context? | Verdict |
|----------|----------|---------|-----------------|---------|
| `http://127.0.0.1` (current) | ✅ | ❌ (loopback port) | ✅ localStorage/getUserMedia/routing all work | shipping default |
| `file://` (extract to temp dir) | ✅ | ✅ | ❌ **not secure** → breaks getUserMedia/geo, localStorage unreliable | inadequate |
| inline via `SetHtml` | ✅ | ✅ | ❌ `about:blank` opaque origin, hash-routing only | inadequate |
| **native `goleo://` (registered secure)** | ✅ | ✅ | ✅ | **the real answer — needs glaze changes** |

**Why it needs a glaze fork (glaze exposes no scheme hook):**
- **macOS:** `WKURLSchemeHandler` must be set on the `WKWebViewConfiguration` **before** the
  `WKWebView` is created. glaze creates the config internally, so — unlike the permission shim,
  which is a post-creation GObject signal we could attach externally — this **cannot** be done from
  goleo; it must live inside glaze (a fork).
- **Linux:** `webkit_web_context_register_uri_scheme` + `webkit_security_manager_register_uri_scheme_as_secure`
  on the WebKitWebContext; the handler builds a `GInputStream` (`g_memory_input_stream_new_from_data`)
  and calls `webkit_uri_scheme_request_finish`. *Possibly* attachable externally via purego (like the
  permission shim), but GTK3/webkit2gtk-4.1 vs GTK4/webkitgtk-6.0 differ, so it's fragile.
- **Windows:** `ICoreWebView2.AddWebResourceRequestedFilter` + `WebResourceRequested`, served from
  the embedded FS — reachable only via go-webview2's `edge.Chromium` (also not exposed by the
  high-level API).

**Decision / recommendation (2026-07-13): deferred, low priority.** Given native IPC already
eliminated the RPC surface, the residual is a loopback-only, embedded-assets-only, token-gated,
origin-allow-listed static server — a small surface. A cross-platform `goleo://` is a substantial,
hardware-gated, three-backend native effort (macOS strictly requires forking glaze). Right sequence
when pursued: add scheme handlers **inside the glaze fork** (the fork tooling already exists —
`scripts/pin-glaze-fork.*`), register the scheme as secure, expose it through glaze's API, then
have goleo serve the embedded FS through it behind an opt-in `Config`. Spike per platform
(Linux/macOS on the CI runners) before wiring. Until then the loopback asset server stays.

### Secure-context gating spike (`spikes/glaze-scheme-secure/`, 2026-07-13)

**Refinement of the above:** serving bytes over a custom scheme is the easy part; the property that
actually gates a `goleo://` is whether the custom origin is a **secure context** (what
`http://127.0.0.1` gives today — `localStorage` / `crypto.subtle` / `getUserMedia` / history
routing). The three backends are **not equal** here, and macOS is the only genuine unknown — so a
spike was built to probe exactly that: load the *same* page from the custom origin on each backend
and have it report `isSecureContext` + a real `localStorage` write + a real `crypto.subtle.digest`.

| Backend | Secure-context mechanism | Fork? | Result |
|---------|--------------------------|-------|--------|
| **Windows/WebView2** | `SetVirtualHostNameToFolderMapping` over `https://` (via `go-webview2` `edge.Chromium`, already a dep) | **No** | ✅ **PASS — real hardware (dev desktop)** |
| **Linux/WebKitGTK GTK3 (webkit2gtk-4.1)** | `webkit_security_manager_register_uri_scheme_as_secure` on the view's context, attached via an **external purego shim** (like the permission shim) | **No** | ✅ **PASS — Docker+xvfb** |
| **Linux/WebKitGTK GTK4 (webkitgtk-6.0)** | same | **No** | ✅ **PASS — Docker+xvfb+dbus** |
| **macOS/WKWebView** | `WKURLSchemeHandler` set on the config **before** init — **no public "register as secure" API** | **Yes** (config frozen at init) | ✅ **PASS — `macos-14` runner** (the gating result) |

**RESULT (2026-07-13): ✅ PASS on all three desktops — the uniform `goleo://` PR is viable.** The
whole `glaze-verify.yml` matrix went green: `glaze-macos-14`, `glaze-ubuntu-latest`,
`glaze-linux-gtk4`, and `glaze-windows-scheme` (after a one-line shell fix — the Windows runner
defaults to PowerShell, so `CGO_ENABLED=0 go build` needed a step `env:` block instead of a bash
prefix; the secure-context test itself had already passed on real Windows hardware locally).

**The gating unknown resolved in our favor: a custom `WKURLSchemeHandler` scheme on real WKWebView
(`macos-14`) reports `isSecureContext === true` with working `localStorage` + `crypto.subtle`.**
Historically such schemes reported `false`; current WebKit grants the secure context. So:
- **Windows** — no fork (`edge.Chromium` vhost API, already a dep).
- **Linux GTK3 + GTK4** — no fork (external purego shim, `register_uri_scheme_as_secure`).
- **macOS** — the sole fork requirement, and now proven worthwhile: a small, upstreamable glaze
  change (set `WKURLSchemeHandler` on the config before `initWithFrame:configuration:`, exposed via
  glaze's API).

Verified locally too: Windows (native) + Linux GTK3/GTK4 (Docker) via `scripts/verify-linux-docker.*`.

### Reference implementation proven through glaze's own API (`glazefork/` + `glazeapi/`, 2026-07-13)

Beyond the raw per-backend probes above, the **actual proposed glaze change** is implemented in
`spikes/glaze-scheme-secure/glazefork/` (glaze v0.0.31 + a `SchemeHandler`/`Options`/`NewWithOptions`
API) and exercised through glaze's own architecture (config/init flow, `Bind`, run loop) by
`spikes/glaze-scheme-secure/glazeapi/`:
- **macOS:** `WKURLSchemeHandler` set on the config **before** `initWithFrame:configuration:` — the
  one piece that *must* live inside glaze. ✅ **PASS on `macos-14`** (`glazeapischeme` green): the
  fork approach — not just raw purego — gives `isSecureContext===true` on real WKWebView.
- **Linux:** registers on the view's `WebKitWebContext` + `register_uri_scheme_as_secure`, serving
  from an in-memory `GInputStream`. ✅ PASS GTK3 (local Docker) + on CI.
- **Windows:** `NewWithOptions` added for API uniformity; scheme wiring is a documented **upstream
  TODO** (goleo uses `jchv/go-webview2` on Windows, which already exposes the vhost API, so this
  gap does not gate goleo).

**IMPLEMENTED (2026-07-13): `Config.SchemeAssets` ships for macOS + Linux.** The glaze scheme API
was pushed to the fork (`daforester/glaze` `v0.0.32-goleo.2`, branch `goleo-scheme`) and goleo pinned
to it (`scripts/pin-glaze-fork.*`). `runtime/scheme_assets.go` + `newGlazeWebView`
(`webview_glaze.go`) serve the embedded FS over `goleo://` when `Config.SchemeAssets` is set; Windows
returns `webviewSupportsSchemeAssets()==false` and falls back to loopback (its `go-webview2` wrapper
needs the vhost rework — follow-up). Verified end-to-end on Linux GTK3+GTK4 (Docker) **and `macos-14`** via
`spikes/goleo-scheme-verify` (`goleo://app` reports `isSecureContext` + localStorage + WebCrypto over
native IPC, no TCP port) — the full `glaze-verify.yml` matrix is green including the goleo integration
(not just glaze in isolation). Downstream consumers need the fork `replace` (Go replaces don't
transit), so `goleo new` scaffolds it. Upstream issue: `GLAZE_ISSUE.md`.
**Remaining: Windows `SchemeAssets`** still falls back to loopback (`go-webview2` needs a vhost hook —
see below).

**Conclusion — the all-platforms `goleo://` is fully de-risked.** goleo consumes glaze's macOS
scheme path from a pinned fork (`scripts/pin-glaze-fork.*`; upstream issue drafted in
`spikes/glaze-scheme-secure/GLAZE_ISSUE.md`), uses `go-webview2`'s vhost on Windows, and a runtime
purego shim (or the same forked glaze) on Linux. **Decision (2026-07-13): keep Windows on
`go-webview2`** — glaze's WebView2 backend would make the scheme feature *more* work (COM rewrite),
force re-verifying the proven Windows stack (native IPC, in-process multi-window), and reintroduce
purego on Windows (the one platform currently free of the `fakecgo`/systray link risk). Unifying on
glaze remains a possible *separate* future migration, evaluated on its own merits.

## Finding — macOS: glaze + gogpu/systray `fakecgo` link collision (2026-07-13)

**Symptom (found by the `macos-14` runner):** linking any executable that pulls in **both** glaze
(the webview) and the tray fails on macOS:
`link: duplicated definition of symbol _cgo_init, from go-webgpu/goffi/internal/fakecgo and
ebitengine/purego/internal/fakecgo`.

**Cause:** glaze uses `ebitengine/purego`; `gogpu/systray` uses `go-webgpu/goffi`. Both ship a
`fakecgo` shim (both forked from Ebitengine) that exports `_cgo_init`. The **Mach-O** linker rejects
the duplicate; the **ELF** linker (Linux) tolerates it — so the tray + glaze link and run fine on
Linux (proven: `glaze-runtime-verify` PASSED on Linux with the tray linked), and Windows is
unaffected (it uses go-webview2, no purego). **macOS-only.**

**Why it slipped past earlier checks:** it is a *link*-time error. `go build ./runtime/...` compiles
a library and never links, so it passed for darwin; only building an actual executable
(`glaze-runtime-verify`) surfaced it — first on the runner, then reproduced locally by
**cross-linking** for darwin from Windows (`CGO_ENABLED=0 GOOS=darwin go build -o x .`). Lesson:
cross-*link* an executable per target, not just `build ./...`.

**Fix (2026-07-13) — the tray now works on macOS too, via purego/objc.** Rather than drop the tray
on macOS, `tray_darwin.go` implements it directly on `ebitengine/purego` + the Objective-C runtime
(`NSStatusItem` + `NSMenu`, menu-bar-only `accessory` activation policy) — the **same FFI glaze
uses**, so it shares glaze's single `fakecgo` and never imports `gogpu/systray`/`goffi`. Result: the
darwin dep tree has **zero** goffi/systray, so no `_cgo_init` collision. `tray_desktop.go` is
`!darwin && !mobilebuild && !js` (systray on Windows/Linux, unchanged); `TraySupported()` is **true**
on macOS again. **Verified on real hardware:** the `glaze-tray-verify` smoke (build a tray, run the
native loop, self-quit) **PASSED on `macos-14`** (Apple Silicon, the objc/NSStatusItem backend) and
on Linux (Docker/systray). (Dedup of the two byte-identical fakecgo copies was rejected — gutting
goffi's exports breaks its FFI.) So the system tray is now cgo-free and hardware-verified on all
three desktops.

## Windows → glaze migration: unify on one webview binding (2026-07-14) ✅ DONE + verified

**Decision (revisited):** with the glaze scheme PR forked/pinned anyway, keeping a *second*
webview binding (`jchv/go-webview2`) on Windows costs more than it saves. Moved Windows onto the
**glaze** backend (WebView2 via purego, same fork as macOS/Linux) so goleo carries ONE cgo-free
binding for all three desktops. go-webview2 kept one release behind `-tags goleo_webview2`.

**De-risked first (local, real Windows):**
- **`_cgo_init`/fakecgo link:** an exe linking glaze (purego) + `gogpu/systray` (goffi) — the
  collision that fails on macOS Mach-O — **links fine on Windows PE** (like Linux ELF). Windows was
  previously the one platform free of purego; now it has it, and PE tolerates the dup.
- **glaze WebView2 `Bind` round-trip** works on real WebView2 (the native-IPC primitive).

**Migration + verification (all on real Windows, glaze backend):**
- Native IPC ✅; **in-process multi-window** (`inProcWindowManager`, per-`LockOSThread` goroutine)
  ✅ (2nd window opened via `OpenWindow` round-tripped over its own native channel); **scheme
  assets** ✅ (`https://goleo.localhost` secure — see the goleo:// section); **tray** ✅
  (`glaze-tray-verify`); **clean Quit** ✅.
- **Lifecycle bug fixed:** `App.Run` unblocked the primary window by `runtime.GOOS=="windows" →
  Destroy()`, which was really a *backend* assumption (go-webview2's Destroy posts WMClose). glaze's
  Destroy doesn't post WM_QUIT, so on glaze-Windows Quit hung ~30s. Replaced with a per-backend
  `endRunLoop()` (glaze/cgo `Terminate()`, go-webview2 `Destroy()`).
- **Not a permission regression:** neither go-webview2 nor glaze auto-grants WebView2
  media/geolocation on Windows today. glaze's vtbl exposes `AddPermissionRequested`, so wiring an
  auto-grant (the Windows analog of the Linux permission shim) is a possible follow-up.

**Follow-ups done (2026-07-14):** **Windows permission auto-grant** — a
`PermissionRequested`→Allow COM handler in the glaze fork's WebView2 backend
(v0.0.32-goleo.3); getUserMedia now returns a verdict on real WebView2 instead of
hanging (kept off the upstream scheme PR — it's goleo policy). **go-webview2 dropped**
entirely — glaze is the sole Windows binding (`runtime/webview_windows.go` + the dep
removed).

**Native menu bar on glaze-Windows — ✅ visually confirmed working (2026-07-14)** (user-verified on a
GUI build; the user32 `HMENU` + WndProc subclass hooks glaze's HWND correctly). **So the
Windows→glaze migration is fully verified end-to-end** — webview, native IPC, custom-scheme assets,
in-process multi-window, tray, native menu bar, permission auto-grant, and clean Quit all confirmed
on real WebView2. Nothing outstanding.

## Upstream glaze PR — scheme handler API, maintainer review round (2026-07-18)

The maintainer accepted the custom-scheme PR's shape (issue #27) and asked for a
round of changes before merge; all addressed and pushed to
`daforester/glaze` `upstream-scheme` (the clean PR branch, based on
`crgimenes:master`, no goleo-only permission grant):

- **Windows:** `AddRef` the `ICoreWebView2Environment` when stored + `Release`
  in `Destroy` (it is used later, at request time, by
  `CreateWebResourceResponse`) — it was held without a reference. And
  reconstruct the canonical `<scheme>://<authority>/path` URL for the handler
  (the https vhost drops the authority, so it is remembered at `Navigate` time)
  + preserve the URL fragment on `Navigate` — so `SchemeRequest.URL` has ONE
  format on all three backends. Kept the hardware-proven `https://<scheme>.localhost`
  vhost/filter rather than a subdomain encoding (which would need a fresh
  Windows hardware pass).
- **Linux:** `g_memdup2` (GLib ≥ 2.68) resolved via `Dlsym` with a fallback to
  `g_memdup` on older GLib (Debian 11 / Ubuntu 20.04) — `RegisterLibFunc`
  panics on a missing symbol. `registerSchemes` now returns an error (nil
  context / security manager / missing lib) instead of silently leaving a
  scheme unregistered; `NewWithOptions` propagates it.
- **macOS:** non-nil `NSError` (`NSURLErrorFileDoesNotExist`) on the not-found
  path (a nil can raise); autorelease the per-request `NSHTTPURLResponse` and
  unregister the `WKURLSchemeHandler` delegates in `Destroy` (two leaks).
- **Docs/tests:** README "Custom URL schemes" section + runnable
  `examples/scheme`; extended the Windows scheme unit test (fragment,
  reconstruction, fallback, round-trip).

Verified from the Windows host the way CI checks it: `CGO_ENABLED=0` build for
darwin/linux/windows × amd64/arm64 (lib + examples), `go vet` + `golangci-lint`
v2.12.2 per GOOS, `gofmt`, and the scheme unit tests (real GUI behavior is the
CI runners' job — can't drive it headless). Draft reply in
`spikes/glaze-scheme-secure/REVIEW_REPLY.md`.

**goleo consumption:** the same source fixes were ported onto the
`goleo-scheme` line (which additionally carries the Windows permission
auto-grant), tagged **`v0.0.32-goleo.4`**, and goleo re-pinned/re-vendored to
it (`scripts/pin-glaze-fork.ps1 github.com/daforester/glaze v0.0.32-goleo.4`).
The example/README are omitted there (examples module is not vendored; its
import path differs on the fork).

**Follow-up review round (2026-07-20) — the merge blocker.** The maintainer
found one more Linux divergence: a `nil` `SchemeHandler` response (documented as
"not found") was finished with an empty stream + default MIME — a *successful
empty document* — whereas macOS uses `didFailWithError:` and Windows returns a
default 404. Fixed to call `webkit_uri_scheme_request_finish_error()` with a
`G_IO_ERROR_NOT_FOUND` `GError` on the nil path (`webview_linux.go`). Pushed to
`upstream-scheme`; ported to `goleo-scheme` → tag **`v0.0.32-goleo.5`**; goleo
re-pinned/re-vendored and shipped as goleo **0.2.8**. With that, the maintainer
said the PR is good to merge.

**PR merged upstream (2026-07-20); what's left before we can drop the fork.** The
scheme API now lives in `crgimenes/glaze` master. The fork is still required for
two reasons: (1) upstream has not cut a *tagged release* containing it (latest is
`v0.0.31`), and (2) the **Windows WebView2 permission auto-grant** (fork commit
`953debd`) was deliberately kept out of the PR — it's goleo policy, not a safe
library default. That grant can't move to goleo's runtime the way the Linux
permission shim did: glaze's `Window()` returns the HWND, not the `ICoreWebView2`
(a private field with no accessor), and WebView2 has no HWND→interface recovery —
so there's no external hook to attach `add_PermissionRequested` to. The clean
path off the fork is an upstream **permission-request hook** (host decides
allow/deny; goleo supplies an auto-allow callback) — issue drafted in
`spikes/glaze-scheme-secure/PERMISSION_HOOK_ISSUE.md`. Meanwhile the bridge's
`getUserMedia` camera fallback is now time-bounded (`bridge/src/camera.ts`) so an
unanswerable prompt fails cleanly instead of hanging — defense-in-depth, not a
grant substitute.

## Cross-cutting testing learnings (not "spikes" but hard-won)

- **CI mobile guard must target GOOS=android/ios, never the host.** `linux + mobilebuild` is an
  unreal combination that trips cgo-only desktop files (`camera_linux.go` under `CGO_ENABLED=0`)
  and says nothing about mobile safety. Real gomobile compile set = `GOOS=android`/`ios`
  `-tags mobilebuild`.
- **Android dev must serve the UI over a secure context.** `goleo emulate android` loading from
  `http://10.0.2.2` (not a secure context) silently disables the WebView's secure-context-only
  APIs — `getUserMedia`/camera, clipboard, geolocation. Production (`http://127.0.0.1`) is
  secure and works. Fix: serve dev over `http://localhost` via `adb reverse` → the whole class
  works in emulation. (Discovered via "clipboard doesn't work on Android".)
- **A cgo webview + `CGO_ENABLED=0` are mutually exclusive** — the root cause behind several
  findings above; the cgo-free bindings are what let goleo keep `CGO_ENABLED=0`.
- **A vendored project can't build for mobile in vendor mode.** Scaffolded projects commit a
  `vendor/` (offline desktop builds + pinned glaze fork), which makes Go auto-select
  `-mod=vendor`. But `gomobile bind` pulls in `golang.org/x/mobile`'s bind-support packages that
  `vendor/` does not contain (they're only reached through gomobile's *generated* code), and
  `go get -tool …/gobind` outright refuses to run under `-mod=vendor`. So the mobile build path
  forces `GOFLAGS=-mod=mod` on every `go`/`gomobile` invocation (`modModEnv`/`goToolEnv`/
  `setMobileEnv` in `cli/cmd`), resolving those deps from the module cache instead. `go mod tidy`
  is unaffected (it always ignores `-mod`). Verified: `goleo build android` on a freshly
  scaffolded+vendored project failed with `gomobile: missing golang.org/x/mobile dependency`
  until this fix, then produced a working APK.
- **The mobile native shell is a fixed all-providers shell, so its provider bindings must
  always be bound.** `MainActivity.java` (and `AppDelegate.swift`) unconditionally import and wire
  every native provider (`gomobile.BatteryProvider`, `Gomobile.setBLEProvider(...)`, …). But the
  gomobile provider files (`backend/gomobile/battery.go`, …) and their `runtime.Set*Provider`
  re-exports are gated `//go:build mobilebuild && goleo_<feature>`, and `mobileBindTags` originally
  only enabled a feature tag when the app called its `Register*`. So the **default scaffold** (all
  `Register*` commented out) — and any app not enabling *every* feature — failed to build with
  `error: cannot find symbol gomobile.BatteryProvider`: the shell references a binding gobind never
  generated. Fix: `mobileBindTags` now always includes the 8 native-shell provider tags
  (`nativeShellProviderTags`: battery, wakelock, sensors, background, nfc, ble, clipboard, share),
  so the provider↔native bridge is always bound; per-feature *bridge-command registration* stays
  opt-in via `Register*` (detected as before) and manifest permissions stay scan-driven. iOS wires
  7 of the 8 (no NFC/BLE) — forcing all 8 is a harmless superset there. **Verified:** `goleo build
  android` on a scaffold went from this compile error to a working APK once the tags were forced.
- **Icon generation is pure Go.** A single `bundle.icon` PNG (recommended 1024²) is downscaled
  (area-averaging in premultiplied-alpha space) and re-encoded into every platform artifact —
  multi-size Windows `.ico`, macOS `.icns`, Linux hicolor PNG + `.desktop`, Android
  `mipmap-*/ic_launcher(+_round).png`, iOS `AppIcon.appiconset` — with no ImageMagick / iconutil /
  external tooling (`cli/cmd/icons.go`, unit-tested in `icons_test.go`). Mobile icons are only
  wired into the manifest/xcodegen when a source icon resolves, so a project without one keeps the
  platform default rather than referencing a missing resource.
- **Never pass user data as a trailing argv element to `powershell -Command`** (fixed 2026-07-31;
  bug present through v0.3.0). `runtime/clipboard`'s Windows write was
  `exec.Command("powershell", "-NoProfile", "-Command", "Set-Clipboard", text)`. Go's
  `syscall.EscapeArg` quotes an argument containing spaces, but **powershell.exe re-parses its own
  command line for `-Command` and strips that quoting**, so `Set-Clipboard hello world` bound two
  positional args and the write failed outright: *"A positional parameter cannot be found that
  accepts argument 'world'."* So **copy-to-clipboard was broken for every string containing a
  space** — i.e. almost all of them — and text starting with `-` silently bound to a parameter name
  instead. Two related corruptions in the same file: `strings.TrimSpace` on read destroyed
  leading/trailing whitespace on *all three* desktops, and on Linux an empty clipboard surfaced
  xclip's `target STRING not available` as a hard error where Windows/macOS return `""`.
  **Fix:** Windows now calls the Win32 clipboard API directly (`OpenClipboard`/`EmptyClipboard`/
  `SetClipboardData` with `CF_UNICODETEXT`, `RtlMoveMemory` into a `GMEM_MOVEABLE` handle,
  `LockOSThread` because the clipboard is owned by the opening *thread*) — no subprocess, no
  quoting, no console-codepage question; the package was split into `clipboard_{windows,darwin,
  linux,unix,mobile,stub}.go` to match `runtime/battery`'s shape. Two Win32 details worth keeping:
  after a successful `SetClipboardData` the **system** owns the handle (freeing it corrupts the
  clipboard — only free on the failure path), and reading must copy out of the locked block
  *before* `CloseClipboard`. Passing the locked address to `RtlMoveMemory` rather than converting
  it to an `unsafe.Pointer` in Go also keeps `go vet`'s `unsafeptr` check quiet.
  **Verified:** `runtime/clipboard/clipboard_test.go` round-trips 15 payloads (spaces, quotes,
  backticks, `$(…)`, `;`/`|`/`&&`, leading `-`, newlines, unicode, empty, whitespace-only) —
  green on real Windows **and** on Linux/xclip under Docker+xvfb. macOS is compile-verified only
  (pbcopy/pbpaste already took the payload on stdin, so they were never affected by the quoting
  bug). Note CI ran **no** `go test` step at all when this was written, so these tests only ran
  locally. **Superseded 2026-08-03:** `ci.yml` now has a `go test ./...` step — safe on a bare
  runner because this package's round-trip skips without `xclip`. Adding it immediately paid for
  itself by surfacing six `cli/cmd` android_deps tests that had been failing permanently on Windows.
  Sibling shell-outs (`runtime/dialogs`, `runtime/notify`) pass one whole script string as the
  single `-Command` argument, which is the safe shape — they were not affected.

---

## Fork slimmed: rebased onto upstream `v0.0.46`, scheme API dropped (2026-08-03)

**Upstream released the scheme API.** `crgimenes/glaze` has moved `v0.0.31` → **`v0.0.46`**, and the
merged custom-scheme PR now ships in a tagged release (`scheme.go`, `webview2_scheme_windows.go`,
plus the darwin/linux handlers — the full `Options`/`NewWithOptions`/`SchemeHandler`/`SchemeRequest`/
`SchemeResponse` surface goleo calls). That closes the first of the two conditions the 2026-07-20
entry above listed for retiring the fork.

**The fork is still required — but its delta is now one commit.** Rebased
`daforester/glaze` onto `v0.0.46` carrying only the WebView2 permission auto-grant, tagged
**`v0.0.46-goleo.1`**; goleo re-pinned + re-vendored. Total delta vs upstream is **53 insertions /
1 deletion across 4 files**: `go.mod`'s module line + `webview2_permissions_windows.go` + its
wiring in `webview2_windows.go`/`webview_windows.go`. Everything the fork used to carry for the
scheme API now comes from upstream. `scripts/pin-glaze-fork.*` defaults bumped `v0.0.31` → `v0.0.46`,
and the scaffold templates (`cli/cmd/templates.go`, `cli/cmd/templates/demo/go.mod.tmpl`) re-pointed
at the new tag.

**Re-confirmed the grant still can't move into goleo's runtime** (so the fork can't be dropped
outright): upstream v0.0.46 declares the `AddPermissionRequested`/`RemovePermissionRequested` vtable
slots, but wires **no** handler and exposes **no** permission hook — and every COM type
(`iCoreWebView2`, `asWebView2`, …) is still unexported with no accessor. `Window()` returns the HWND
only, and WebView2 has no HWND→interface recovery, so there remains no external attachment point for
a COM event on `ICoreWebView2`. The remaining path off the fork is unchanged: an upstream
permission-request *hook* (`spikes/glaze-scheme-secure/PERMISSION_HOOK_ISSUE.md`).

**A subtlety that bites consumers — do NOT repoint upstream's test imports at the fork.** The first
rebase attempt also rewrote `editor_scenario_darwin_test.go`'s `github.com/crgimenes/glaze/editor`
import to `daforester/…` for internal consistency. That **broke `go mod tidy` in goleo**:
`go mod tidy` walks the *test* imports of dependencies (`github.com/crgimenes/glaze tested by
…glaze.test imports github.com/daforester/glaze/editor`), and because the consumer's `replace` maps
`crgimenes/glaze` — not `daforester/glaze` — that import resolved as a *separate external module*,
whose `@latest` (`v0.0.32-goleo.5`) has no `editor` package: `module … found, but does not contain
package`. Left as upstream's path it resolves through the consumer's own replace and tidies clean.
So: change **only** `go.mod`'s module line; leave every other upstream file byte-identical.

**Verified (from the Windows host, `CGO_ENABLED=0`):** `go mod tidy` + `go mod vendor` clean;
`go vet ./runtime/... ./cli/...` clean; and — per the cross-link/fakecgo lesson above — a real executable
importing **both** the webview (glaze/purego) and the tray was **cross-linked**, not just compiled,
for all six desktop targets (windows,linux,darwin × amd64,arm64): all six link, and `runtime/cgo` is
absent from the dep tree on each. That link check matters here specifically because v0.0.46 bumped
`purego` `v0.10.1` → `v0.10.2` — the exact library whose `fakecgo` `_cgo_init` export collides with
`gogpu/systray`'s `goffi` on Mach-O. It does not collide. Real-GUI behavior (the grant actually
firing on WebView2) is unchanged code on an unchanged path, but has **not** been re-driven on
hardware in this pass.

---

## Spike — the REAL `@goleo/bridge` in a REAL webview (2026-08-04) ✅ PASS on Windows/WebView2

**The gap.** `bridge/src/*.test.ts` (55 tests) drives the bridge against a `FakeSocket`;
`runtime/ws_e2e_test.go` + `nativeipc_test.go` drive the wire format from hand-written Go frames.
Both suites are green **while neither has ever executed the two halves together**, so three things
had no coverage at all:

1. **Native transport selection.** `bridge.ts` prefers `window.__GOLEO_NATIVE__` and falls back to
   WebSocket. Every TS test stubs global `WebSocket`, so they *always* took the fallback branch —
   the native path, which is the **default** for a desktop window with `Config.NativeIPC`, was
   never run by a test.
2. **That the base64 binary encoding agrees.** Go uses `base64.StdEncoding`; `fs.ts` uses its own
   chunked `bytesToBase64`/`base64ToBytes`. Each side's tests assert its own half. Nothing checked
   they share an alphabet, padding and chunking — and this was broken *in both directions* before
   Phase 3, which is exactly the bug class two green suites cannot see.
3. **That a backend error reaches the page as its own text.** The `fs.ts` rethrow (real error when
   connected, "requires the Go backend" only when absent) shipped unexecuted.

**`spikes/bridge-e2e-verify/`** closes it: one goleo app with `NativeIPC` + `SchemeAssets` (so **no
TCP port**), whose page loads the package's actual `tsc` output as **ES modules with no bundler**.
Checks: the native channel was chosen; a 23-byte payload that is deliberately **not valid UTF-8**
(PNG magic, `0xFE 0xFF 0x80 0x7F`, a lone surrogate half) survives page→Go→page byte-exact; a write
outside the FS roots is refused **and leaves no file**; the page receives the *backend's* confinement
message; `appDataDir()`'s grant makes a write succeed.

**Mutation-verified (four, each restored):** masking errors in `fs.ts` → "fs.ts masked a real backend
error"; `bytesToBase64` returning `TextDecoder` output (the literal pre-Phase-3 bug) → `illegal base64
data at input byte 0`; unpadded base64 → `illegal base64 data at input byte 28`; `checkFSPath`
short-circuited → *"the denied write actually created …"*. Breaking native detection → `native:false`
plus `backend not connected`, which also proved the page's raw-`__goleoSend` escape hatch works when
the bridge itself is unusable.

**Two real defects found by building it:**
- **The published `@goleo/bridge` was not loadable without a bundler.** `tsc` (`moduleResolution:
  bundler`) emitted extensionless relative imports (`from './bridge'`), which no browser and no Node
  ESM loader can resolve — Vite papered over it for template users. All 63 specifiers across 28 files
  now carry `.js`; `node -e "import('./dist/index.js')"` works, and the spike loads it over
  `<script type="module">` with no bundler. Keeping it bundler-free is deliberate: if that ever
  regresses, the spike fails.
- **No way to observe which transport won.** `Bridge.native` is private, so the native path could not
  be asserted from outside — a plausible reason it went untested. Added `isNative()` (+ a top-level
  export and three unit tests, mutation-checked).

**Cross-cutting lesson — `//go:embed` makes verification loops lie.** `main.go` embeds
`frontend/dist`, so rebuilding the page **without recompiling the binary** leaves the old page inside
the old executable and the run silently verifies stale code. This produced three false PASSes while
mutation-testing, each indistinguishable from success. Two shell traps compounded it: `tsc` failing
on a now-unused symbol meant the mutation never got built (silenced by `>/dev/null`), and
`node prepare.mjs | tail -1` masked a non-zero exit so `&&` continued with a stale artifact — use
`set -o pipefail` and don't silence builds inside a verification loop. `prepare.mjs` now runs
`go build` itself, making the staleness structurally impossible rather than something to remember.

**CI:** added to `glaze-verify.yml` on the mac/Linux matrix arm *and* the Windows job — worth both
because WebView2 serves the assets over the `https://goleo.localhost` vhost while WKWebView/WebKitGTK
serve the literal `goleo://`, so the page reaches its modules by different routes.

**Hardware results (2026-08-04): ✅ Windows/WebView2 (local, `https://goleo.localhost`) and
✅ ubuntu-latest/WebKitGTK (CI, `goleo://app`)** — both byte-exact on the binary round-trip, both
enforcing confinement with the backend's own error. macOS is still pending (its first run failed on
the toolchain issue below, not on the spike).

**First CI run also exposed a runner-image dependency:** the step passed on ubuntu-latest and failed
on windows-latest + macos-14 with `'tsc' is not recognized`. The Ubuntu image ships a **global
`typescript`**; the others do not, so the step was silently relying on a runner-provided global.
`prepare.mjs` now installs the bridge's devDependencies when absent — which also means a fresh clone
can run the spike. Lesson: a step that shells out to a JS tool must install it, and a green Ubuntu
arm is not evidence the other two have the tool.

**And the real-socket tests from the same batch found a genuine data race** in the Phase 3 per-server
hub: `Server.Stop` read `s.hub` directly (`if s.hub != nil`) while a WebSocket upgrade created it
lazily under `hubOnce`, so an ordinary shutdown-with-a-client-connecting was an unsynchronised
read/write. It reproduced on **Linux CI only** and passed on Windows, i.e. it was found by luck of
interleaving — `runtime/ws_e2e_test.go`'s `TestHubInitAndStopDoNotRace` now forces the overlap and
reproduces it on Windows too. Fixed by routing `Stop` through `clients()`.

---

## Cross-cutting — the dev path hides the user path (2026-08-04)

Three user-visible bugs shipped for one structural reason, so it is worth stating
plainly: **`goleo new` behaves differently when you are developing goleo than when
a user runs it, and only the development path was ever exercised.**

`GOLEO_ROOT` swaps the goleo module for the local checkout via a `replace`, and
`linkBridge()` npm-links a local `@goleo/bridge` over the frontend's dependency. So
neither of the two things a real user *resolves* — the Go module version from the
proxy, the npm range from the registry — is resolved at all in a dev scaffold. The
only CI job that scaffolds (`mobile-verify.yml`) sets `GOLEO_ROOT` **and**
`--no-install`, so npm never ran either.

What that hid:

1. **`go: inconsistent vendoring` in a fresh install (v0.8.1–v0.8.8).** Three layers
   deep into stale `optionalDependencies`. Found by a user, in production.
2. **`frontend/package.json` pinned `"@goleo/bridge": "^0.2.1"`.** A caret on a `0.x`
   version locks the minor, so it resolved to 0.2.9 against a v0.8.x runtime —
   binary file I/O broken, confinement errors misattributed. The Go require beside
   it had been fixed long before; the npm pin one line away was missed.
3. **`goleo build` on Windows wrote `app` with no `.exe`.** The `current` target took
   the host's `GOOS` but hardcoded `OutputExt: ""`. Windows cannot execute it. The
   default command, on the most common desktop, producing an unrunnable artifact.

Each was invisible to a green CI. Note also that the existing `TestBinaryOutputName`
constructed **synthetic** `buildTarget` values, so it passed while the real `targets`
map was wrong — testing the helper, not the table.

**Structural fix: `.github/workflows/release-smoke.yml`.** It installs the published
`@goleo/cli` from npm on windows/ubuntu/macos-14, scaffolds **without `GOLEO_ROOT`**,
runs a real `npm install`, builds, and asserts the artifact's name and executability
per-OS plus that both pins match the release. It has no `actions/checkout` at all —
anything read from the repo could mask the packaging bug it exists to catch. It runs
*after* publishing, because the published artifacts are the subject.

Its ESM-loadability step **fails against 0.8.8 and earlier by design** (they shipped
extensionless relative imports); verified locally against published 0.8.8.

Local reproduction of the whole user path, for future reference: build the CLI with
`-ldflags "-X …/cli/cmd.Version=<published>"`, then run `goleo new` with
`env -u GOLEO_ROOT`, `npm install`, and `goleo build`. That sequence found bug 3.

---

## Phase 4 — Android signed release, verified on a real emulator (2026-08-04) ✅ PASS

`goleo build android --release` and the derived-permission manifest, checked end to end on
an Android 36 x86_64 emulator with a signed release APK built from the demo scaffold.

**What the device confirmed** (`adb shell dumpsys package com.goleo.app`):

- **Signed:** `apkSigningVersion=2` with a real signature block. The signing path works —
  build.gradle.kts reading the credentials from the environment, Gradle applying the
  signingConfig, and the artifact coming out signed rather than as the `-unsigned` variant.
- **Version wiring:** `versionCode=100`, `versionName=0.1.0`. Both were hardcoded `1` and
  `"1.0"` in the template before Phase 4, so goleo.json's values were loaded and thrown
  away. 0.1.0 → 100 is the derived `major*10000+minor*100+patch`.
- **Permissions:** exactly the 14 the demo enables, each attributable to a feature. A
  minimal scaffold gets 3.
- **The `maxSdkVersion=30` bound works:** `BLUETOOTH` and `BLUETOOTH_ADMIN` are in the
  generated manifest but ABSENT from the installed package on API 36 — the platform drops
  them, which is precisely the "unnecessary permission" report the bound exists to avoid.

**Where the extra device-side entries come from**, since the count does not match the 14
and that looked wrong at first: `RECEIVE_BOOT_COMPLETED`, `BIND_JOB_SERVICE` and `DUMP`
are declared by AndroidX (WorkManager) and arrive via manifest merging;
`ACCESS_COARSE_LOCATION` is added by the platform itself when an app requests
`ACCESS_FINE_LOCATION`. Nothing leaks from goleo's derivation — checked by diffing the
generated manifest against `intermediates/merged_manifest/release/.../AndroidManifest.xml`
against the installed package.

**And the full runtime chain**, which is what a false-negative permission would break:
launched the app, navigated to the Camera demo, tapped Start camera →
`GrantPermissionsActivity` appeared with *"Allow permcheck to take pictures and record
video?"* → granted → `CAMERA: granted=true` and a **live preview rendering the emulator's
synthetic scene**, with the device label (`camera 0, facing front`) appearing only after
the grant. So: derived manifest permission → Android prompt → grant →
`WebChromeClient.onPermissionRequest` → `getUserMedia` → frames.

**Two rough edges found by doing this rather than by testing:**
- `keytool` is not on PATH even when `JAVA_HOME` is correct, because it lives in the JDK's
  `bin/`. Added `goleo generate android-key`, which uses the JDK goleo already resolves for
  Gradle. Writing it reintroduced the flag-confusion bug fixed twice elsewhere in this file:
  a base64 password beginning with `-` was parsed by keytool as an option, producing a
  keystore whose password was not the one printed. Passwords are hex now, and the keystore
  is opened with the reported password before it is reported.
- A trailing space in `GOLEO_ANDROID_KEYSTORE` (which `set VAR=path ` in cmd.exe keeps)
  failed 37 seconds in at `:app:packageRelease` as `Trailing char < > at index 141`, naming
  neither the variable nor the space. Now trimmed in the CLI and in build.gradle.kts, and
  the path is checked for existence up front.

**Still unverified:** a Play internal-track upload. That needs a developer account and is
the only remaining confirmation that Play accepts the AAB and does not flag the permission
set. CI (`mobile-verify`'s `android-release` job) covers the AAB build, jarsigner
verification, the manifest being minimal, and that `--no-sign` really produces something
unsigned.

---

## Cross-cutting — "accepted and ignored" is its own bug class (2026-08-04)

Four flags and one whole template were accepted by the CLI and then not read. They are
worth recording together because the shape repeats and because **each one reported
success**, which is what makes the class expensive: the user gets an artifact that is
wrong in a way nothing announces.

- **`goleo build ios --release`** — exempted from the mobile flag check in anticipation of
  an `.ipa` export that does not exist, so `buildForIOS` never read it. You asked for a
  release artifact, got a debug build, and the build printed a success line.
- **`--android-ndk`** — declared on **both** `build` and `emulate`, documented in `--help`
  as "Path to Android NDK", read by **nothing**: resolution went straight to
  `ANDROID_NDK_HOME` and autodiscovery. So naming an NDK silently built against a
  different one, and when none was found the error said *"Set ANDROID_NDK_HOME manually"*
  with the flag sitting unused in the help output. `docs/guide/01-installation.md` had
  documented it as a working alternative the whole time.
- **`--android-abi` on a non-Android target** — accepted, ignored.
- **`goleo emulate android -o NAME`** — declared as "Output APK name" into the *build*
  command's `buildOutput` global. The dev APK is built inside `.goleo/android-dev/` and
  `adb install`ed straight from there, so there was never an artifact for a name to apply
  to. Removed rather than implemented.
- **`mobile.ios.bundle_identifier` / `deployment_target`** — parsed into the config struct
  in 0.9.0 and then discarded; `xcodegen.yml` took the bundle id from the **Android**
  `package_name`. `MIGRATING.md` had announced them as working.

**Why the whole class survived: `runBuild`'s flag-rejection block had no test at all.**
Extracted as `validateTargetFlags` and covered by an exhaustive *flag × target* table, so
an unwired flag now shows up as an accepted no-op rather than shipping as one. `--no-sign`
is deliberately excluded, with the reasoning recorded beside it: every other flag asks for
something to **happen**, so ignoring it leaves the user with an artifact they did not ask
for, whereas `--no-sign` asks for something **not** to happen and a target with no signing
step satisfies that. There is nothing silently wrong to report.

**A config key can be worse than a missing one.** `--ios-target` defaulted to 14.0 and fed
gomobile while `mobile.ios.deployment_target` defaulted to 15.0 and fed xcodegen — two
independent sources for one value, disagreeing with **no configuration at all**. It never
showed because a framework minimum *below* the app's is harmless; the failing direction is
above, so lowering `deployment_target` to 13.0 made Xcode refuse the link citing iOS 14.0,
a version the user never chose. Now one source (`cli/cmd/mobile_minversion.go`), with the
flags as explicit overrides. The invariant is asserted by rendering the template and
comparing, not by restating the expected value.

**Making that config-driven turned a latent divergence into a build failure**, which is
worth noting as a hazard of this kind of fix: `android-dev/app/build.gradle.kts` hardcoded
`minSdk 24` while the release template read `min_sdk`. Harmless while `-androidapi` was
also hardcoded 24; once gomobile followed the config, any project raising `min_sdk` above
24 failed on the `goleo emulate` path only — Gradle rejects a library whose `minSdk`
exceeds the app's. Fixed by templating the dev project too, with a test that dev and
release must declare the same levels.

### The frontend was shipped twice in every mobile artifact

`frontend/dist` was copied into the native project (`app/src/main/assets` on Android,
`App/Assets` on iOS) **and** embedded in the Go library via the generated
`backend/gomobile`'s `//go:embed all:frontend/dist`. Only the embedded copy is ever used:
the shells load `http://127.0.0.1:<port>`, which the Go server serves from `EmbedFS` — and
it has to be loopback, because that is a secure context and `file:///android_asset` is not.
Confirmed in a real APK before the fix: `assets/index.html`, `assets/manifest.json`,
`assets/sw.js` and 18 `assets/assets/*.js` chunks, ~130 KB uncompressed for the demo app,
scaling with the frontend. Nothing in `MainActivity.java` or `AppDelegate.swift` referenced
any of it.

Verified after removal on a real Windows host: `goleo build android` produces an APK with
**no** `assets/` entries, while the exact hashed chunk name from that same build
(`index-C8-X-0Q5.js`) is present inside `lib/x86_64/libgojni.so` — so the copy the server
actually serves is intact and the removed one was redundant. A test fails if a shell starts
referencing native assets while the copy is gone, since the reference and the copy have to
return together.

**A measurement trap while doing this:** the pre-existing `app.apk` in the test project was
a `--release` build (an `app.aab` sat beside it), so comparing its size against a fresh
**debug** APK was meaningless — the new one was *larger*. Different build types, different
R8/dexopt output, and a `baseline.prof` present in one and not the other. Sizes are only
comparable across identical build types at the same revision.

### Mutation testing lies when the pattern does not match the line endings

`cli/cmd/android_deps.go` is **CRLF**; most of `cli/cmd` is LF. A mutation applied with a
multi-line `\n` pattern silently matched nothing, the test passed, and that read as "the
test does not catch this" — the inverse of the truth. It was caught only because the
expected failure message never appeared, prompting a match-count check.

This is the same family as the `//go:embed` staleness already recorded above: **a
verification loop that can silently do nothing will eventually tell you something false.**
Rule: every mutation must assert it applied (`assert s.count(old) == 1`) before the test is
run, and must detect the file's own line ending rather than assuming `\n`.

### iOS tier 1, first CI run: the generated project was unopenable (2026-08-04)

**The load-bearing probe passed** — `gomobile bind -target=ios,iossimulator` does emit an
`ios-arm64-simulator` slice, so the whole simulator approach is sound. The job then failed
three steps later at `xcodebuild`:

```
xcodebuild: error: Unable to read project 'GoleoApp.xcodeproj'
  Reason: The project cannot be opened because it is in a future Xcode project
          file format (77).                                    exit status 74
```

**Cause: XcodeGen's default project format tracks the newest Xcode, not the user's.**
`options.projectFormat` defaults to `xcode16_0`, which writes pbxproj `objectVersion` 77;
anything older than Xcode 16.0 refuses to open it. `brew install xcodegen` on the runner
gave a current XcodeGen while the `macos-14` image's Xcode is older. Verified against
XcodeGen's own `ProjectFormat.swift`: `xcode14_0`=56, `xcode15_0`=60, `xcode15_3`=63,
`xcode16_0`=77 (the default), `xcode16_3`=90 — and **an unrecognised value falls back to 77
silently**, so a typo reintroduces the failure with no error of its own.

**Not just a CI problem.** goleo tells users to open `.goleo/ios/` in Xcode for device
builds, so the generated project has to be readable by the Xcode they have — and nothing in
it needs a new format (they are plain build settings). Pinned to **`xcode14_0`**: newer
Xcodes read older formats, so the oldest sufficient format is also the most compatible.

Three things came out of this beyond the one-line pin:
- **`explainXcodebuildFailure`** (`cli/cmd/xcodebuild_errors.go`) — `xcodebuild failed: exit
  status 74` named neither xcodegen, nor the generated file, nor the Xcode version needed.
  The failure is only in xcodebuild's prose, so its output is now tee'd through a capped
  buffer and matched. Unknown objectVersions are still named but **not** mapped to an
  invented Xcode version.
- **CI records `xcodebuild -version` and `xcodegen --version`** before building, and asserts
  the generated `project.pbxproj` really is objectVersion 56. Both halves of this mismatch
  are moving targets supplied by the runner image, so the next one should be readable from
  the log rather than guessed at.
- **Lesson: a step that `brew install`s a tool has taken a dependency on that tool tracking
  something other than the runner.** This is the same shape as the earlier finding that the
  Ubuntu image ships a global `typescript` while the others do not — a toolchain the job did
  not pin is a toolchain that will move.

### The generated-header dump: two names, and neither was guessable (2026-08-04/05)

Third macOS run. The project-format and framework-name fixes held — `xcodebuild` read the
project, found `Goleo.xcframework` (both `ios-arm64` and `ios-arm64_x86_64-simulator` slices
present, confirming the probe on a real build) — and the Swift compile failed, as expected.

The job printed the generated `Gomobile.objc.h`, which settled what no amount of reading
`AppDelegate.swift` could:

- **The Swift MODULE is `Goleo`** — gomobile titlecases the `-o` basename
  (`bind_iosapp.go`: `name = base minus ".xcframework"; title = strings.Title(name)`;
  `Module: title`). So `import Goleo` was right all along.
- **Every SYMBOL is prefixed `Gomobile`** — gobind derives the Objective-C prefix from the Go
  **package** name (`backend/gomobile`), not the module: `GomobileSetHomeDir`,
  `GomobileStartServer`, `GomobileSetBatteryProvider`, `GomobileNotifierProtocol`.
- **Package-level Go funcs become C functions**, so they take no argument labels:
  `GomobileEmitSensorReading(t, x, y, z, ts)`, not `emitSensorReading(x:y:z:timestamp:)`.
  `AppDelegate.swift` had `Goleo.emitSensorReading("accelerometer", x: …, y: …, z: …,
  timestamp: …)` — wrong on the prefix, the namespace *and* the labels.
- **Each Go interface generates a protocol AND a same-named wrapper class**, which is why
  Swift appends `Protocol` to the protocol. That half of the original guess was correct.

So the shell used the module name as if it were both the prefix and a namespace. Every method
*shape* it declared (`show(_:body:)`, `share(_:text:url:)`, `startSensor(_:) throws`,
`readText() -> String`) matched the header exactly — only the naming was wrong.

**A truncation that read as a finding.** The dump capped at `head -80`, which cut the
`@interface` list after the fourth entry — alphabetically `BLEProvider, BackgroundProvider,
BatteryProvider, ClipboardProvider`. That looks exactly like "only four interfaces have a
wrapper class", which would mean five of the nine protocols do *not* get the `Protocol` suffix
in Swift. Caught by noticing the output was exactly 80 lines and stopped at header line 266,
then confirming both the forward declarations and the `@interface` blocks are alphabetical and
cut at the cap. The step now prints the declaration **count** before truncating, so the next
run states the number instead of implying it. Same lesson as elsewhere in this file: a silent
cap looks like complete coverage.

**And a test that passed vacuously.** `TestIOSShellUsesTheGeneratedBindingNames` checked
`strings.Contains(src, "import Goleo")` — but the explanatory comment added to
`AppDelegate.swift` in the same change also contains those words, so deleting the real import
left the test green. Found by mutating the import away and getting no failure. It now matches
the import **statement** with `(?m)^\s*import Goleo\s*$`.

### The Swift compiled; the asset catalog did not (2026-08-05)

Fourth macOS run. **`AppDelegate.swift` compiled** — the binding names read off the generated
header were right, and every method shape it already declared matched. One error left, and it
was not in any goleo-authored line:

```
Assets.xcassets: error: None of the input catalogs contained a matching
                        ... app icon set named "AppIcon".
```

**Cause: a `{{if .HasIcon}}` gate that can add a setting but not prevent one.** goleo
deliberately omits `ASSETCATALOG_COMPILER_APPICON_NAME` when no `bundle.icon` resolves, so the
app keeps the platform default — and it correctly omitted it here. But **XcodeGen applies its
own `settingPresets` to an application target, and those include
`ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon`.** So the project asked `actool` for an icon set
goleo had deliberately not generated. The fix is an explicit **empty** value in the else
branch, which overrides the preset and drops `actool`'s `--app-icon`.

Note the blast radius: the scaffold's `goleo.json` points `bundle.icon` at `assets/icon.png`
and **the scaffold does not create that file**, so `HasIcon` is false by default. This was
every new project's first iOS build, not an edge case. The same shape as the Android
launcher-icon gating, but inverted — there the risk was referencing a missing resource; here it
was a *third party* referencing one on goleo's behalf.

**A comment that was executable.** The explanation added above the fix contained a literal
`{{if .HasIcon}}`. `extractMobileTemplate` runs every file through `text/template`, so a
`{{...}}` inside a YAML comment is still an action — it opened an unterminated `if` and broke
the entire file with `unexpected EOF`. Caught immediately because the new test renders the
template, but nothing else would have: the file is only rendered during a real mobile build.
`TestEveryMobileTemplateParses` now parses all 23 mobile templates, so the class is covered
rather than the instance.

### ✅ iOS builds, installs and RUNS (2026-08-05) — the full bridge on a simulator

Fifth run, on a `spike/ios-build` branch. **Green end to end.** From a freshly scaffolded
project on a `macos-14` runner: `GoleoApp.app` built for the Simulator, bundle verified
(objectVersion 56, `CFBundleName` "iosapp", version 0.1.0, LaunchScreen present), installed,
launched, and still running with a live PID after 15s.

The screenshot is the part that matters, because a live process is not a working app. It shows:
- the **embedded UI rendered** — WKWebView loaded the frontend the Go server serves over
  loopback;
- `{"os":"ios","arch":"arm64","name":"iOS"}` — `goleo:getOS` round-tripped **from the Go
  backend**;
- *"Backend says: Hello, Goleo! From Go backend at 2026-08-05T00:05:23Z"* — a custom invoke;
- *"Backend Event: heartbeat … {"goroutines":10,…}"* — **server→client push events**.

So on iOS: gomobile-hosted Go backend, loopback asset serving, WKWebView, bridge invoke, and
event push all work. `runtime/`'s mobile path is genuinely exercised on iOS for the first time.

**Five defects, each only reachable once the previous was fixed** — the shape of bringing up a
path that had never executed. In order: (1) the Xcode project format, (2) the xcframework name,
(3) the gomobile binding names, (4) XcodeGen's app-icon preset, and (5) — found by reading
rather than running — the BGTask identifier taken from the Android package name, which
`BGTaskScheduler.register` would have thrown on during `didFinishLaunching`. Only (5) was
invisible to CI, because the default scaffold's two identifiers happen to be equal.

**On runner cost, and a prediction that did not hold.** macOS bills at 10x. The failing run was
179s (~30 billable min) and reaching it also ran `ci` plus two Android jobs, so the work moved
to a branch with a single-job workflow: no simulator-slice probe (a whole throwaway
`gomobile bind`, 29s), a Go module/build cache, and `cancel-in-progress`. I predicted ~18-20
billable minutes. **The green run cost 51.** The estimate was wrong because the failing baseline
skipped the boot-and-launch steps entirely — they cost 118s, and they only run when the build
succeeds. The honest numbers: a *build-failure* iteration went 179s → ~150s; a *successful* run
is inherently ~300s because booting a simulator and launching an app is expensive. The probe
removal and the cache were real savings; the baseline was not comparable.

The lesson worth keeping: **a green run and a red run are not the same amount of work**, so
"cost per iteration" measured on failures understates the finish line.

## Play upload — the permission report reconciled, and one real bug (2026-08-05)

**First ever Play upload** (internal track, throwaway package `qvbnxwtz.rmpldskg`). Play reported
**19 permissions and 6 features**; the build printed **14 permissions**. Every line reconciles,
and one of the differences was a genuine defect.

**Permissions: 19 = goleo's 16 + 2 merged + 1 from Play.**
- goleo's manifest declares **16**: the 14 the build prints, plus `BLUETOOTH` and
  `BLUETOOTH_ADMIN` carrying `maxSdkVersion="30"`. Those two are deliberately not in the printed
  list (they are the legacy bound) but Play's Bundle explorer shows every `<uses-permission>`
  regardless of `maxSdkVersion`. They still vanish on API 31+ *devices* — confirmed earlier on
  the API 36 emulator.
- AndroidX manifest-merges **2** more: `RECEIVE_BOOT_COMPLETED` (WorkManager) and
  `<package>.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION` (androidx.core, signature-level,
  namespaced to the app).
- **Play itself** adds `com.android.vending.CHECK_LICENSE` — verified absent from the merged
  manifest in `intermediates/merged_manifest/`, so it comes from Play's bundle processing, not
  from goleo or AndroidX.
- `ACCESS_COARSE_LOCATION` is correctly **absent**: the platform adds it at install time
  alongside `ACCESS_FINE_LOCATION`, which is why it appeared in `dumpsys` but not here.

**Features: 6 against 3 declared — and this was the bug.** goleo declared `camera`, `nfc`,
`bluetooth_le`, all `required="false"`. Play reported those plus `android.hardware.bluetooth`,
`android.hardware.location` and `android.hardware.faketouch`.

**aapt2 derives `<uses-feature>` from `<uses-permission>`, and an implied entry defaults to
`required="true"`.** So `BLUETOOTH`/`BLUETOOTH_ADMIN` implied a **required**
`android.hardware.bluetooth`, and `ACCESS_FINE_LOCATION` implied a **required**
`android.hardware.location` — hard device filters on Play, hiding the app from any device
without Bluetooth or GPS, for features that degrade gracefully to a browser fallback. The
`required="false"` work was in place but covered only the features goleo declared *directly*,
not the ones its own permissions implied. `faketouch` is a baseline every device satisfies and is
left alone.

Fixed: `goleo_ble` now also declares `android.hardware.bluetooth`, and `goleo_geolocation`
declares `android.hardware.location` (+ `.gps` defensively — Play reported only the former, but
Android's documented table lists both and aapt's implied set has changed across versions;
declaring an inert `required="false"` line is cheaper than losing device coverage). The demo now
emits six optional features, matching what Play derives, and the build prints them.

`TestEveryImpliedHardwareFeatureIsDeclaredOptional` guards the **class**, not the instance: it
walks a permission→implied-feature table against the full feature set and fails if any implied
feature is undeclared. Enabling a new feature whose permission implies hardware and forgetting
the `<uses-feature>` is otherwise invisible — the printed permission list looks completely
correct.

**Outcome (2026-08-05, same day): the corrected bundle was accepted.** goleo 0.10.4,
versionCode 101, signed, internal track. Play reported exactly the predicted **7 features** —
the six goleo now declares `required="false"` plus `faketouch`, which the platform implies and
which every device satisfies — and **19 permissions**, unchanged, confirming the fix touched
features only. **19,041 supported Android devices** out of a catalogue of roughly 20k models.

**But there is no before/after on that figure**, because the device count for versionCode 100
was never captured. 19,041 is consistent with the fix having widened reach *and* with it having
made no measurable difference on this permission set: requiring `bluetooth` and `location`
excludes only models with no Bluetooth radio and no location provider at all, a thin slice of
the catalogue. The pre-fix bundle is still readable (*Bundle explorer -> versionCode 100 ->
device compatibility*) if the comparison is ever wanted. Note also that Play's Features list
renders required and optional entries identically, so the list matching the prediction confirms
the manifest arrived intact but is NOT independent evidence the two are optional — that came
from `required="false"` in the release *merged* manifest, checked locally.

So the fix rests on **correctness, not on the device number**. The same defect on a more
commonly-absent implied feature would have excluded a large fraction of the catalogue, with no
local symptom then either.

**Why nothing caught this before Play:** the emulator check confirmed the *permissions* were
right and that the `maxSdkVersion` bound worked, and CI asserts the manifest is minimal. Neither
looks at implied features, because they exist only after aapt2 processes the manifest, and their
consequence is a store-side distribution filter with no local symptom at all. This is the one
finding in the whole Android effort that genuinely required a real store to surface.

### The GOLEO_ROOT replace outlives the variable (2026-08-05)

Found while updating the test project to the released 0.10.4. Its `go.mod` said
`require github.com/daforester/goleo v0.9.3` — but also carried
`replace github.com/daforester/goleo => E:/Development/goleo`, so it had been compiling the
working tree for days while the require was **decorative**. Bumping the require alone would have
looked like an upgrade and changed nothing.

**Cause:** `ensureLocalReplace` is called from six places but `snapshotModFiles` guards only the
mobile and emulate paths. So `goleo build` (desktop) and `goleo dev` inject the `GOLEO_ROOT`
replace and never remove it — **the effect outlives the environment variable**, and every later
build silently uses the checkout whether the variable is set or not.

This is the same family as the "dev path hides the user path" entry above, and it had a concrete
cost here: a verification reported as being against the released module was not.

**Fix: warn, do not auto-remove** (`cli/cmd/replace_warn.go`). The replace is legitimate and
wanted while developing goleo; what is indefensible is it being invisible. When `go.mod` replaces
goleo with a **directory** and `GOLEO_ROOT` is **not** set, the build now says so and prints the
two commands that undo it.

Two implementation details worth keeping:
- **Distinguish a directory replacement from a module replacement by the absence of a version**,
  which is Go's own rule, rather than pattern-matching the path — that would have to cope with
  `./`, `../`, `/abs` and Windows drive letters. A fork pin
  (`=> github.com/someone/goleo v0.10.4`) is deliberate and must never warn.
- **Do not match a fixed `"<module> =>"` string.** The first version did, so a hand-formatted
  `go.mod` with extra whitespace made the warning silently not fire — caught by its own test
  case. Splitting on the arrow and comparing fields is whitespace-proof, and the block form
  (`replace ( … )`, which `go mod edit` never writes but a human might) is handled too. **A
  warning that silently fails to fire is worse than no warning, because silence reads as "all
  clear".**

---

## Cross-cutting — publish order decides propagation risk (2026-08-05)

`release-smoke`'s **windows-latest** job went red on 0.10.5 with

```
npm error code ETARGET
npm error notarget No matching version found for @goleo/bridge@^0.10.5
```

The release was fine. Every assertion that ran passed; the five reported "failures" were steps
**skipped** downstream of one hard step losing a race, and ubuntu and macOS won it.

**The compounding, which is the transferable part.** `release.yml` published `@goleo/bridge`
**last**, and `release-smoke` resolves it **last** too (install the CLI, scaffold, *then*
npm-install the frontend). So the least-propagated package had the latest deadline. Worse, it was
the only network-resolving step **without a retry** — the CLI install had 4×30s and the scaffold
2×60s, both for packages published *earlier*. The step with the least slack was the one with no
safety net. Timings: `release` finished 14:49:41, smoke started 14:49:45 — four seconds later —
and npm still 404'd the version at 14:50:44, ~63s in.

**Root-cause fix: publish `@goleo/bridge` FIRST.** Its slot was arbitrary — verified it has no
dependencies and no dependents among the published packages; the only genuine constraint is that
the platform packages precede `@goleo/cli` so its `optionalDependencies` resolve. A retry on the
frontend `npm install` went in as defence in depth.

**Validated on the 0.10.6 release, and the retry never fired** — `npm install` succeeded first
attempt, so the reorder *eliminated* the race rather than narrowing it. Worth stating because the
opposite result was equally possible and would have meant the retry was load-bearing.

Rule worth keeping: **the package a consumer resolves LAST should be published FIRST.** And when
a job retries some network steps but not others, the unretried one is where it will fail.

Diagnostic note: 0.10.5 was re-verified by dispatching `release-smoke` against it *after*
propagation completed — green on all three, same code, later in time. That is what turned a
plausible story into a confirmed one; a fix shipped on the story alone would have been a guess.

## Cross-cutting — a skip-guard must not be able to hide the defect (2026-08-05)

`runtime/clipboard`'s round-trip matrix goes through the system clipboard, a **global
single-owner resource**. On a real Windows machine roughly half of all round trips came back
empty, and — decisively — the failing SET **shifted between runs**. That is contention (a
clipboard manager, RDP session or Office instance taking ownership between the write and the
read), not a defect: no clipboard code had changed since the release that passed. `go test ./...`
was red for reasons unrelated to the code, which is worse than it sounds, because that noise can
bury a genuine clipboard regression.

The obvious fix — skip when the round trip fails — would hide exactly the quoting and trimming
regressions the matrix was written to catch. The guard is therefore **asymmetric**:

| canaries passing | meaning | action |
|---|---|---|
| all 8 | machine is quiet | run the matrix strictly |
| some | contended | **skip**, naming the cause |
| none | a write that never lands is a real bug | **do not skip** — fall through and fail |

Per payload it retries up to 4× but **only on an empty read**, which is what theft looks like. A
non-empty mismatch is corruption and fails immediately. Exhausting the retries still fails, so a
read that always trims — precisely what `"   "` → `""` is — fails every attempt.

Mutation-verified all three branches: a no-op write FAILS (0/8 canaries, no skip), a mangled read
fails immediately as corruption, an alternating-empty read SKIPS with the diagnostic. **A guard
added to reduce flakiness must be mutation-tested against the bug it could mask**, or it is just
a way of not being told.

## iOS — the first real-device run, and the four defects it found (2026-08-09)

First execution of the demo scaffold on physical hardware: **iPhone 17 Pro Max (17,2), iOS 26.6,
Xcode 26.6 (17F113), macOS 26.5.2 (25F84), Go 1.26.5 darwin/arm64, XcodeGen 2.46.0.** It built,
launched and rendered its UI. Ten of fourteen checklist items passed on first contact —
accelerometer, gyroscope, magnetometer, battery, clipboard both directions, wake lock, background
sync registration, camera and location permission prompts. What follows is the other four, all of
which are now fixed but **none of which are verified on hardware yet**.

Read this before concluding any of the four is over-engineered; each has a mechanism that a
simulator run does not exercise.

**1. Notifications: permission granted, nothing ever delivered.** iOS suppresses a notification
whose app is in the foreground unless a `UNUserNotificationCenterDelegate` implements
`willPresent` and asks for it. `GoleoNotifier.show` posts with `trigger: nil`, so every
notification fires immediately — i.e. always in the foreground while testing. No delegate was set,
so the default (present nothing) applied. The permission prompt working is what made this read as
a delivery failure rather than a presentation one. `add(request)` also discarded its completion
handler, so a rejection failed in total silence.

**2. Dialogs had no mobile provider at all — on iOS *and* Android.** `runtime.SetDialogsProvider`
existed; no `backend/gomobile` binding did, and neither shell called one. Every `goleo:dialog*`
call on either platform returned `no native provider registered`. **A provider nobody registers
is a valid program**, so nothing caught it at build time, and the source comment claiming the JS
bridge would fall back to `window.alert` was wrong — `bridge/src/dialogs.ts` gates fallbacks on
`backendPresent()`, and on mobile the backend is always present. Detail in
`docs/agents/host-features.md`; the parts worth knowing here are that provider methods must pass
**JSON strings** (gobind omits, rather than rejects, a reverse-bound method it cannot bind) and
that `goleo_dialog` had to join `nativeShellProviderTags` — the shells wire it unconditionally, so
without the forced tag any app that does not call `RegisterDialogs` fails to compile its shell.
That failure is invisible in the demo scaffold, which enables everything, and breaks the minimal
one, which is the `goleo new` default.

**3. The share sheet silently did nothing.** Unlike (2), the provider *was* registered. It
resolved its presenter through `UIApplication.shared.windows`, deprecated since iOS 15 and empty
under the scene lifecycle — and the device log carries `UIScene lifecycle will soon be required`.
`root` was nil, `present` was a no-op, and `ShareProvider.share` returns void, so Go never learned
anything had failed. Presenters now resolve through `GoleoUI`, which is handed the app's own
window in `didFinishLaunching`.

**4. `goleo build ios` could not build for a device at all, for two independent reasons.** The
checklist's device run came from Xcode, not the CLI; `build-device.log` ends in failure.
  - No `DEVELOPMENT_TEAM`, and no config or flag to supply one — so xcodebuild stopped at
    `Signing for "App" requires a development team`. The obvious workaround, selecting a team in
    Xcode, does not survive: goleo regenerates `.goleo/ios/` on every build. Now
    `mobile.ios.development_team` / `--ios-team`, refused early by `validateIOSSigning` rather
    than after a full gomobile bind.
  - **No `-destination`.** xcodebuild silently takes the first matching destination, and on a Mac
    with "Designed for iPad" support that is `{ platform:macOS, variant:Designed for
    [iPad,iPhone], name:My Mac }` — so the iOS device target built a **Mac** app and reported
    success. The only trace is one warning among hundreds of lines: `Using the first of multiple
    matching destinations`. Now pinned to `-sdk iphoneos -destination generic/platform=iOS`.

Benign, recorded so they are not re-investigated: the `com.apple.CoreMotion.plist` permission
warning, `Could not create a sandbox extension`, and `xpc_user_sessions_get_foreground_uid failed`
are all normal for a sandboxed WKWebView app. The duplicated `starting up...` line comes from a
single `log.Println`, so it is stderr duplication rather than double initialisation — unconfirmed,
and harmless either way.

Microphone was the one checklist item marked `?`. It is not a defect: `NSMicrophoneUsageDescription`
is in the Info.plist template and the `WKUIDelegate` grants capture. It was simply never exercised.

**Found while implementing the dialogs provider, not on the device:** a blocking modal and
the bridge's concurrency model interact badly. Every invoke runs on **its own goroutine**
(`runtime/websocket.go`), so a frontend that fires two dialogs without awaiting the first
runs them concurrently — and the file picker on both platforms parks its result in a single
slot shared with the activity / app-delegate callback. One call would consume the other's
result and the loser would block forever, since these methods deliberately have no timeout.
Both shells now serialise dialog presentation and refuse to run on the UI thread. Worth
recording because the same shape applies to any future provider that blocks for a user
answer: **"only one can be on screen" is not the same as "only one can be in flight."**

**Re-running this:** `docs/ios-device-verification.md` is the hand-over sheet — setup,
the eighteen checks, and what to do when the generated Swift protocol signatures disagree
with what was written blind.

**The general lesson, which is why this entry is long:** every one of (1), (2) and (3) is a
*silent* failure — a suppressed notification, an unregistered provider, a nil presenter. None
raised an exception, none failed a build, and the simulator run that preceded this one
(`BUILD SUCCEEDED`) said nothing about any of them. Mobile host features cannot be validated by
compilation; the checklist is the test.

## Mobile — the WebView permission gates were not gates (2026-08-10)

Follow-up review after the iOS device spike, looking for the same *shape* of defect
elsewhere rather than the same symptom. The native shells decide camera, microphone and
geolocation on behalf of the WebView, and **neither shell restricts navigation** — both let
the WebView follow any link — so those gates answer for whatever page it reaches, not just
for the app's own UI. Three separate holes:

1. **iOS granted everything, unconditionally.** `GoleoWebPermissionDelegate` received
   `origin` in both callbacks and ignored it, calling `decisionHandler(.grant)` outright.
2. **Android matched a string prefix.** `request.getOrigin().toString().startsWith(
   "http://127.0.0.1")` — and `http://127.0.0.1.evil.com` is an ordinary registrable domain
   that satisfies it. The dev shell had the same bug spelled `http://localhost`.
3. **Android's geolocation callback had no origin check at all**, so any page could read the
   device's location as long as the app itself held the runtime permission. The camera/mic
   path next to it was gated; this one simply was not.

All three now parse the origin and compare the **host for equality**, which is what
`devOriginAllowed` in `runtime/server.go` already did — the Go side was right and the shells
had each invented their own weaker version. `10.0.2.2` (the Android emulator's alias for the
host's loopback) is accepted by the **dev** shell only; a test asserts the release shell does
not trust it.

Worth noting how this was found, because the symptom was invisible: nothing failed, no log
line appeared, and every checklist item still passed. It came from asking "where else does
this codebase decide something on behalf of a caller it did not authenticate?" — the same
question the `goleo:openURL` scheme allow-list and the WS origin allow-list already answer.
**A permission check that is never observed failing is not evidence that it works.**

The corresponding regression check on hardware is that camera and location STILL prompt —
the gate is new code in the path of two checks that previously passed.

## Mobile — gobind's (value, error) result does not bind to Swift (2026-08-10)

**0.10.7 shipped an iOS build that does not compile.** The dialogs provider was introduced
with Go methods shaped `XxxJSON(optsJSON string) (string, error)`. That compiles fine for
Android — both `android` and `android-release` jobs were green — and fails on iOS with

	type 'GoleoDialogs' does not conform to protocol 'GomobileDialogsProviderProtocol'

The reason, read out of gomobile's own generator rather than guessed
(`bind/genobjc.go` in `x/mobile@v0.0.0-20260803200217`):

- the 2-result case (line ~507) sets `s.ret = g.objcType(typ)`,
- `objcType` maps a Go `string` to **`NSString* _Nonnull`** (line ~1322),
- so the emitted declaration is
  `-(NSString* _Nonnull)openFileJSON:(NSString* _Nullable)optsJSON error:(NSError**)error`.

Swift only rewrites a trailing `NSError**` into a throwing method when the result can
express failure. A `_Nonnull` object cannot, so **no Swift signature conforms** — not
`throws -> String`, not `-> String`. The method is unimplementable from Swift.

gomobile's own source comment on that branch says *"Return is nullable, so satisfies the
ObjC/Swift error protocol"* — but the annotation it emits says `_Nonnull`. The comment
describes an intent the code does not carry out, which is why reading the comment (or
reasoning from it) leads you straight into the wall.

**The rule to keep:** a reverse-bound provider method may return a value **or** an error,
never both. Both shapes already in this repo are safe — `ClipboardProvider.ReadText() string`
(lone value) and `SensorsProvider.StartSensor(string) error` (error only, which becomes
`BOOL` + `NSError**` and imports as `throws`). Dialogs now returns a lone string and carries
failures inside a JSON envelope: `{"error":...}` / `{"value":...}` / `{"paths":[...]}`.
`TestDialogsProviderIsWiredEndToEnd` fails the build if an error result comes back.

Two process notes worth as much as the finding:

- **Android green is not evidence for iOS.** The same interface bound cleanly for Java and
  was unimplementable in Swift. Any change to a provider interface needs the
  `ios-simulator` job, not just the Android ones.
- The `mobile-verify` step named *"Generated Objective-C API surface (ground truth for
  AppDelegate.swift)"* grepped only `@protocol|@interface|FOUNDATION_EXPORT`, so it printed
  which protocols exist and **not their method signatures** — the one thing it is there to
  establish. It confirmed `GomobileDialogsProvider` existed while saying nothing about the
  shape that was wrong.

  **Fixed the same day.** The step now prints each protocol body in full, and flags any
  method matching `_Nonnull)<name>:… error:(NSError` as unimplementable from Swift, naming
  the cause instead of leaving "does not conform to protocol" to be decoded. Both halves
  were checked against a synthetic header before landing: the detector fires on the 0.10.7
  shape and stays quiet on both working ones — a lone value (`readText`) and `BOOL` +
  `NSError**` (`startSensor`, which is how a Go error-only result imports as `throws`).

## Demo scaffold — nothing compiled it, so a checklist item was untestable (2026-08-10)

The iOS device checklist asked for a microphone permission prompt and got "?" both times.
The reason was not iOS: **no demo page could reach the microphone.** `CameraDemo.vue` passed
`audio: false` explicitly and nothing else touched audio — while both shells carried live
audio-capture permission branches (`RESOURCE_AUDIO_CAPTURE` on Android, the `WKUIDelegate` on
iOS) and the iOS `Info.plist` declared `NSMicrophoneUsageDescription` with no feature behind
it. Live code, a declared purpose string, and no way to exercise either.

Fixed by making Microphone its own opt-in feature (`RegisterMicrophone`, `goleo_microphone`)
rather than folding `RECORD_AUDIO` into Camera — most camera apps only want stills, and that
permission is one users see and Play flags. Detail in `docs/agents/host-features.md`.

**The structural finding is the one to keep.** The demo scaffold's Vue pages were compiled by
**nothing**: `mobile-verify` scaffolds the MINIMAL template, and the demo's `.vue` files live
under `cli/cmd/templates/` where no tsconfig reaches them. A broken demo page — a bad import,
a registry entry pointing at a file that does not exist, a typo in an SFC — would only
surface when a user ran `goleo new --demo`. `ci.yml` now has a `demo-scaffold` job that
scaffolds the demo against the working tree and builds **both** halves: `vite build` (parses
every SFC, resolves every import), `vue-tsc --noEmit` (types), and `go build ./...` (the
backend, which gains a `Register*` call whenever a feature is added).

Two things that cost time and are worth not rediscovering:

- `npx vue-tsc@2` does **not** work. npx installs its own TypeScript, vue-tsc 2.x requires
  `typescript/lib/tsc`, and recent TS no longer exports that subpath — it dies with
  `ERR_PACKAGE_PATH_NOT_EXPORTED` before checking a single file. Install it into the project
  so it uses the template's own `typescript ^5.3`.
- The bridge must be **built** before scaffolding: `goleo new` npm-links the local checkout
  when `GOLEO_ROOT` is set, but the package resolves to `dist/`, so without `npm run
  build:bridge` every bridge import in every demo page is unresolved.

Both the frontend build and the type-check were run locally against a real scaffold before
the job was added, precisely so the gate would not land red on pre-existing errors. It is
clean today.

## Android — `goleo emulate android` passed API level 0 to gomobile (2026-08-10)

Reported from a real run: `goleo emulate android` on NDK 28 failed with

	ANDROID_NDK_HOME specifies .../ndk/28.2.13676358, which is unusable:
	unsupported API version 0 (not in 21..35)

Nothing is wrong with that NDK. `emulate.go` passed the raw `androidAPI` global to
`gomobile bind -androidapi`. That global is **`goleo build`'s `--android-api` flag**, and
`emulate` does not declare that flag at all — so it was always **0**. gomobile validates the
level against the NDK's `meta/platforms.json` (present since NDK r23, so every current NDK)
and refuses anything outside 21..35.

`goleo build android` was never affected: it resolves via `resolveAndroidMinAPI(androidAPI,
cfg.MinSDK)` at both of its call sites. Two paths assembling the same argument list, one of
them resolving and one not — which is also why "Android is emulator-verified" in this repo
stayed true while the emulate path was broken.

The message is worth noting on its own: it names the **NDK**, so it reads as a broken SDK
install. Nothing in it points at goleo passing 0, and the obvious response — reinstalling
the NDK, or pinning an older one — cannot work.

Fixed by resolving identically to `build`. `TestEveryGomobileCallSiteResolvesTheAndroidAPI`
greps both files and fails any `-androidapi` argument that is not `minAPI`; mutation-tested
by deleting the resolve and reverting the argument, which the guard catches by file and
line. A second test pins the default resolution at >= 21, the floor gomobile accepts.

**The general shape, which has now appeared three times this session:** a value resolved
correctly on one path and passed raw on another (this), a provider registered by one shell
and not the other (dialogs), a permission gate applied to one callback and not its
neighbour (geolocation). Duplicated call sites drift; the guard has to check *every* site,
not the one that was broken.

## Android — the emulator zeroes out audio input unless told not to (2026-08-10)

The Microphone demo's permission prompt appeared and was granted on an emulator, and
`getUserMedia({audio:true})` still could not obtain a device. The cause is one missing
launch flag, and the emulator says so itself:

	$ emulator -help-allow-host-audio
	Allows sending of audio from audio input devices. Otherwise, zeroes out audio.

`emulate.go` launched with `-avd <name> -no-snapshot-load` and nothing else, so the guest's
audio input was zeroed. `hw.audioInput=yes` was already set on the AVD — the virtual mic
hardware was never the problem, which is worth knowing because that is where the search
naturally starts.

Now passed for windowed runs, and deliberately NOT in headless mode, where `-no-audio` is
correct for CI and directly contradicts it. `emulatorLaunchArgs` was extracted from
`findDevice` so this is testable at all — the same move `validateTargetFlags` needed, for the
same reason: inline argument construction that nothing can assert on.

**The flag is probed, not assumed.** An unrecognised flag makes the emulator refuse to start,
which would trade a silent microphone for a dead `goleo emulate android`. `-help-<topic>` is
unambiguous — exit 0 plus a description for a known flag, exit 1 and `unknown option: ...`
otherwise — so `androidDeps.supportsHostAudio` just checks the exit status.

Two things this turned up on the way:

- **An AVD''s data directory is not `<name>.avd`.** An AVD is registered by a `<name>.ini`
  file at the AVD home root whose `path=` key points at its data directory; the two diverge
  as soon as an AVD is renamed. The dev machine had `emulator-5562.ini` pointing at
  `Medium_Phone.avd/` — a completely healthy, bootable AVD.

  Read as two mismatched files it looks like corruption, and the first diagnosis here was
  exactly that: "delete the stray .ini". **That would have destroyed the machine''s only AVD
  registration.** The lesson is narrow and worth stating: two files whose names disagree are
  not evidence of corruption until you have read what is *inside* them. `emulator-5562.ini`
  named `Medium_Phone.avd` in its first line.

  goleo assumed `<name>.avd` in `avdConfigPath`, which broke three things at once and none of
  them looked related: `avdStatus` reported "unable to verify its system image", `ensureAVD`
  silently skipped its self-heal (a missing sysdir is treated as "can''t introspect, trust
  it"), and the microphone check advised editing a config.ini at a path that did not exist.
  Now resolved through the `.ini`, with the `<name>.avd` fallback kept for AVDs that have no
  `.ini` at all.
- The first cut of the doctor line read that phantom and advised "add hw.audioInput=yes to
  its config.ini" — a file that does not exist. A status line that gives advice must
  distinguish *no config* from *no key*, or it sends people somewhere empty. `avdConfigPath`
  exists to make that distinction possible; `avdSystemImageSysdir` was generalised into
  `avdConfigValue(name, key)` rather than adding a second config.ini parser.

**Currency note (same day).** The `-allow-host-audio` fix above is verified working and was
still not sufficient. On the dev machine, with the flag confirmed in the AVD's
`emu-launch-params.txt` and `hw.audioInput = true` in its `hardware-qemu.ini`, and an active
host microphone, capture still failed with `NotReadableError: Could not start audio source`.

There is a **third** requirement, and it is a runtime one the CLI cannot reach: Extended
Controls → Microphone → "Virtual microphone uses host audio input". The launch flag permits
the emulator process to use host audio; that toggle routes it into the guest. It defaults off
and does not persist across restarts.

Worth separating carefully, because the first two look like they should be enough and the
evidence for them is easy to find: the flag is in the launch params, the hardware is in
hardware-qemu.ini, and both say yes while the guest still gets nothing. `goleo doctor android`
reports the two it can see and says nothing about the third, which is honest but incomplete
by construction — the state lives inside a running emulator.

**Root cause, found on the device (same day).** The emulator was never the problem. With
`-allow-host-audio` confirmed in the launch params, `hw.audioInput = true` resolved, an active
host microphone, and the Extended Controls toggle enabled, capture still failed — while the
emulator's own voice assist recorded fine. `adb logcat` named it outright:

	W cr_media: Requires MODIFY_AUDIO_SETTINGS and RECORD_AUDIO.
	            No audio device will be available for recording

goleo declared `RECORD_AUDIO` and not `MODIFY_AUDIO_SETTINGS`. Chromium's WebView media stack
checks both before it will enumerate ANY input device. `dumpsys package` confirmed the app
requested only the first.

**Why this was so hard to see, and the lesson worth keeping:** `MODIFY_AUDIO_SETTINGS` is a
*normal* permission — granted at install, no prompt, no UI, nothing in the app to inspect.
Every visible signal said the permissions were fine, because the one you can see
(`RECORD_AUDIO`) was prompted for and approved. That combination — a dangerous permission
that prompts correctly and a normal one that is silently missing — looks exactly like a
hardware or configuration fault, and the investigation went to the emulator three times
before going to logcat.

It also cost two wrong diagnoses that were each documented as fact before being tested: that
the emulator zeroes audio (true, but already fixed and not the cause here), and that the
Extended Controls toggle was the missing piece (unconfirmed; it was enabled and did not help).
**`adb logcat` answered in one command what three rounds of reasoning about the emulator did
not.** For anything WebView-related on Android, read the device log first — `cr_media`,
`AudioRecord` and `AudioFlinger` say precisely what they want.

## Release — release-smoke lost the npm propagation race again, on a different runner (2026-08-10)

`release-smoke` failed on **ubuntu-latest** for 0.10.12 with

	npm error code ETARGET
	npm error notarget No matching version found for @goleo/cli@0.10.12.

while windows and macOS passed. **The release itself was fine** — `npm view @goleo/cli@0.10.12`
resolves, as does `@goleo/cli-linux-x64@0.10.12`. Purely propagation timing: the job runs
seconds after the release workflow and one runner reached an edge that had not caught up.

This is the same race that hit the **windows** job on 0.10.5 (see the entry above, where the
fix was to publish `@goleo/bridge` first because it is resolved last). That fix was correct and
did not cover this: `@goleo/cli` is published **last** and installed **first**, so it has the
least propagation time of anything in the release. Reordering cannot fix that — something has
to be published last.

So the job now **waits for visibility instead of retrying the install**. `npm view` is a cheap
metadata query rather than a full failed install per attempt, so the window widened from
4x30s ≈ 90s to 20x15s = 5 min while producing less noise and usually finishing faster.

It waits on **both** `@goleo/cli` and the runner's own `@goleo/cli-<platform>-<arch>`, because
npm drops an unresolvable **optional** dependency silently — a lagging platform package would
otherwise yield a successful install of a CLI that then reports "no prebuilt binary found".
The package name is computed with `node -p process.platform/process.arch` so it cannot drift
from what `bin/goleo.js` looks for.

Verified before committing: the predicate returns true for the published 0.10.12, false for a
nonexistent version, and true for the linux platform package; the step passes `bash -n`.

**The general point:** a retry loop's window is a guess about someone else's infrastructure,
and this one was tuned on the failure that prompted it (~63s) rather than on the worst case.
Two releases later a different runner exceeded it. Prefer waiting for a cheap positive signal
over retrying an expensive negative one.

**The fix was incomplete, and the reason is structural (same day).** Adding
`MODIFY_AUDIO_SETTINGS` to the Microphone feature fixed `goleo build android` and changed
nothing for `goleo emulate android` — the microphone stayed broken on the emulator through a
released fix that looked complete.

The two Android manifests are built **two different ways**:

- `templates/android/…/AndroidManifest.xml` **renders** `{{range .Perms.Permissions}}`, so it
  follows `resolveAndroidPermissions` automatically.
- `templates/android-dev/…/AndroidManifest.xml` is a **static list**, deliberately a superset
  so every demo page works under `emulate` without re-deriving anything.

Static means it drifts, and nothing pointed at it: `scan.go` is where permissions look like
they live, and it is authoritative for exactly one of the two paths.
`TestDevManifestCoversEveryFeaturePermission` now fails when a derived permission is missing
from the dev manifest.

Writing that test immediately surfaced five more divergences — `WAKE_LOCK`,
`FOREGROUND_SERVICE`, `READ`/`WRITE_EXTERNAL_STORAGE`, `BODY_SENSORS`. All five turned out to
be harmless (the first two arrive via WorkManager's library manifest at merge time; the rest
are inert on modern API levels or unnecessary for what the demos do), so they are listed in
`devManifestMayOmit` **with the reason** rather than bulk-added. A new feature permission with
no entry there fails the test, which forces the same check instead of letting it slide.

**The pattern, now the fourth instance this session:** a value resolved on one path and passed
raw on another, a provider registered by one shell and not the other, a permission gate on one
callback and not its neighbour, and now a permission list derived in one manifest and hardcoded
in the other. Every one was invisible because the path that was checked was the correct one.
When two artifacts serve the same purpose by different mechanisms, the guard has to compare
them to each other — testing either alone proves nothing about the pair.

## Dev setup — the lockfile shadowed every developer''s local build (2026-08-10)

`scripts/setup.ps1` produced a CLI that refused to run:

	[goleo] version mismatch: @goleo/cli is 0.10.12 but the installed
	[goleo] native binary (@goleo/cli-win32-x64) is 0.9.1.

Nothing was stale in the usual sense — this is reproducible from a clean clone.
`cli/npm/package.json` declares the platform packages as `optionalDependencies` so END USERS
get the right binary, and `"latest"` is the committed pin (see RELEASING.md for why exact
versions there broke `npm ci`). But **`package-lock.json` froze what `latest` resolved to**,
and that entry sat at `0.9.1` while the tree was at `0.10.12`. A workspace `npm install` —
which `setup.*` runs — therefore fetched a months-old published binary into the repo''s own
`node_modules`, and `bin/goleo.js` resolved the platform package BEFORE the local build. Every
`scripts/setup.*` run on this repo hit it.

The version guard worked exactly as designed: it caught a genuinely wrong binary. Its ADVICE
was wrong — "delete node_modules and reinstall" reinstates the same 0.9.1 from the lockfile.

`bin/goleo.js` now checks for a local dev build first, gated on `go.mod` sitting beside it.
That gate is what makes reordering safe: three levels up from `bin/` is the repo root in a
checkout and `<prefix>/node_modules` in a published install, and only one of those has a
`go.mod`. Verified both layouts before committing, and `goleo version` now reports the local
build instead of refusing to start. `setup.*` also deletes the pulled-in platform packages,
since a binary that shows up in `npm ls` and is not what runs is confusing on its own.

**The transferable bit:** a lockfile pin is a *snapshot of a floating range*, and `"latest"`
in package.json reads as "always current" while the lockfile quietly says otherwise. The two
disagreeing is invisible until something compares them — here, a version guard that only
exists because a previous release shipped a mismatched pair.

**Confirmed working on Android (2026-08-10).** With the dev-manifest fix in place, the
Microphone demo records and plays back on the emulator. That closes a chain of four separate
causes, each of which fully masked the next: the emulator zeroing audio input
(`-allow-host-audio`), a stale AVD path assumption, the missing `MODIFY_AUDIO_SETTINGS`
permission, and that permission reaching only the derived release manifest and not the static
dev one. The checklist item that started it — "microphone permission prompt appears" — was
untestable at the outset because no demo page could reach the microphone at all.

Desktop is marked `yes` in the demo registry on the same evidence path rather than a separate
run: recording is plain `getUserMedia` + `MediaRecorder` in the webview, and every desktop has
an explicit permission route — Linux auto-grants the WebKitGTK `permission-request` signal
(`webview_glaze_permissions_linux.go`), Windows goes through WebView2 plus the glaze fork''s
auto-grant, macOS through the WKUIDelegate. Only the permission *query* has no desktop
equivalent, and the demo says so inline instead of degrading the page.

## iOS — the same "declared but unread" shape, checked deliberately (2026-08-10)

After the Android dev-manifest split, the obvious question was whether iOS has the same shape.
It does, in one place: `featureRegistry.IOSUsageDescs` declares purpose strings for eight
features and **nothing reads them** — `templates/ios/App/Info.plist` hardcodes its own three
(camera, microphone, location). The registry looks like the source of truth for iOS purpose
strings and is not one.

This is worse than the Android equivalent if it ever bites: a missing purpose string does not
deny the request, iOS **terminates the app** the first time it touches the resource.

**No active bug today.** Every declared-but-absent key was checked individually:

| Key | Why it is currently inert |
|---|---|
| `NSPhotoLibraryUsageDescription` | the dialogs use `UIDocumentPickerViewController`, which reads no photo library |
| `NSDocumentsFolderUsageDescription` | a **macOS** key; no effect on iOS at all |
| `NSMotionUsageDescription` | gates `CMPedometer`/`CMMotionActivityManager`; raw accelerometer/gyro/magnetometer need none, and all three were device-verified working |
| `NSBluetoothAlwaysUsageDescription` | iOS has no CoreBluetooth path — no BLE provider is registered |
| `NFCReaderUsageDescription` | iOS has no CoreNFC path — no NFC provider is registered |

The last two are landmines, not exemptions: implementing either feature on iOS without adding
its string at the same time is a launch-time crash. NFC additionally needs the
`com.apple.developer.nfc.readersession.formats` **entitlement**, which goleo does not generate
at all — the purpose string alone would not be enough.

`TestIOSUsageDescriptionsReachTheInfoPlist` now fails when a feature declares a key the plist
lacks, with those reasons recorded as the allow-list. **The test immediately found one the
manual sweep had missed**: `NFCReaderUsageDescription` has no `NS` prefix, so grepping
`NS*UsageDescription` skips it. Worth remembering — the iOS purpose-string keys are not a
uniformly named set, and eyeballing them is unreliable in a way the comparison is not.

## Release — "latest" fixed a lockfile problem and created a quieter one (2026-08-10)

`package-lock.json` pinned `@goleo/cli-win32-x64` and `-linux-x64` at **0.9.1** while the tree
was at 0.10.12. Every `npm install` in this repo — including the one `scripts/setup.*` runs —
pulled a months-old published binary into `node_modules`, where `bin/goleo.js` found it ahead
of the developer''s own build and refused to run on a version mismatch.

The chain is worth following, because each step was a correct fix for the step before:

1. Committing the **exact** version could not work: a release commits `X.Y.Z` before those
   packages exist on the registry, npm cannot lock an unresolvable version, and being
   **optional** it drops them silently — so `npm ci` refused with `Missing:
   @goleo/cli-darwin-arm64@X.Y.Z from lock file`.
2. So they were committed as **`"latest"`**, which always resolves. That fixed `npm ci`.
3. But `"latest"` is a **floating range**, and a lockfile records what a range resolved to
   *once*. `package.json` said "always current" and `package-lock.json` said `0.9.1`, and only
   the lockfile is consulted. Nothing compares the two, so it drifted silently for four
   releases.

The fix is to declare **none** in the committed tree. `build-platform-packages.js` already
*replaced* the whole `optionalDependencies` object at publish time, so it never needed them
pre-declared — the committed values existed only to keep the lockfile happy, and removing them
keeps it happier: no range to float, no pin to go stale, nothing for `npm ci` to reconcile.

Verified all four properties before committing, since this mechanism has broken twice:

| Property | Result |
|---|---|
| `npm ci` on the committed tree | exit 0 |
| lockfile still names any platform package | none |
| publish guard refuses the unstamped tree | exit 1 |
| `build-platform-packages.js` then stamps all six and the guard passes | exit 0 at 0.10.13 |

**The transferable point:** a floating range in `package.json` and a fixed entry in the
lockfile are two statements about the same dependency that no tool reconciles, and the more
reassuring one (`"latest"`) is the one that is not consulted. If a value must not go stale,
do not express it as a range whose snapshot is stored elsewhere.

## iOS — second device run: two clean builds, and a detection gap they exposed (2026-08-11)

`goleo build ios` (device, signed) and `goleo build ios --simulator` both reached
`** BUILD SUCCEEDED **` on Xcode 17F113 / iPhoneOS 26.5 SDK, and the signed app ran on
hardware. The whole build produced **two** warnings, one of them Apple boilerplate
(`No AppIntents.framework dependency found`). So the interesting output was not an error.

### The finding was a line of ordinary build output, not a failure

The build printed:

```
Detected mobile features: goleo_ble, goleo_nfc, goleo_vibration, goleo_geolocation,
goleo_wakelock, goleo_camera, goleo_sensors, goleo_background, goleo_share,
goleo_clipboard, goleo_dialog, goleo_fs, goleo_battery
```

Thirteen features for a demo scaffold that registers fourteen. **`goleo_microphone` was
missing** — `featureRegistry` had the Microphone entry (both audio permissions, the hardware
feature, the purpose string) but `scanPatterns` had no `RegisterMicrophone\(` pattern, so
`detectFeatureUsage` could never return it. `goleo build android` therefore derived a
manifest with neither `RECORD_AUDIO` nor `MODIFY_AUDIO_SETTINGS` — for the debug `.apk`, the
`--release` `.apk` and the `.aab` alike, since `buildForAndroid` is the only build path.

A `buildAndroidDev()` sibling next to it rendered the static `android-dev` manifest into an
`app-dev.apk`, and **had no caller — not one, ever, back to the initial commit.** Go does not
warn about an unused function, so 120 lines of unreachable code sat next to the live path
implying a dev-vs-release build split that does not exist. It is deleted; the comment on
`buildForAndroid` now records that there is one path and that `android-dev` belongs to
`emulate` alone. Worth noting how it did damage without ever running: it made "the dev
manifest is for dev builds, the derived one is for release builds" look true, which is the
model both ineffective microphone fixes were made under.

**Two Android microphone fixes had already landed, and neither one reached a built app:**

| Commit | What it edited | Effect on a built artifact |
|---|---|---|
| `8a600d2` "microphone needs MODIFY_AUDIO_SETTINGS" | `featureRegistry.Permissions` | **none** — that table is only read for tags the scanner emits, and it never emitted this one |
| `1983378` "the DEV manifest is static, so the audio permission missed emulate" | `templates/android-dev/.../AndroidManifest.xml` | made `goleo emulate android` work; `emulate` is not a build |

So the state going into this run was: `emulate` had both permissions, every `goleo build
android` output had neither, and the microphone was believed fixed on Android. The v0.10.13
release note recorded the inverse — that the dev fix gave `emulate` "the permission the
derived release manifest already had", and that 0.10.12 "has it in one manifest only".
0.10.12 had `MODIFY_AUDIO_SETTINGS` in **zero** manifests; 0.10.13 put it in exactly one, the
one that never ships.

That line exists because of mitigation (2) in `cli/cmd/android_permissions.go`: permissions
follow enablement, detection can produce false negatives, so **the build prints what it
detected**. It worked — on an iOS build, for an Android defect, seventeen commits and five
releases after the feature shipped in 0.10.9.

**The release notes recorded the opposite as fact.** The v0.10.13 message says the dev
manifest fix gave `emulate` "the permission the derived release manifest already had". The
derived release manifest never had it. Nobody checked, because the emulator demo recorded
audio and played it back — the platform where it *was* declared was the platform being
tested, and a release note is not a test.

### Why four separate mechanisms hid it

| Mechanism | Why it looked fine |
|---|---|
| `gomobile bind` | `nativeShellProviderTags` forces `goleo_microphone` in for the shells, so `runtime/microphone` linked and compiled on both platforms |
| `goleo emulate android` | uses the **static** dev manifest, which lists both audio permissions — so the mic demo worked on every emulator run, and `emulate` is where all the Android microphone work was done |
| iOS | `templates/ios/App/Info.plist` hardcodes `NSMicrophoneUsageDescription` and iOS has no manifest permissions at all, so the platform that got a hardware run was structurally immune — **the mic was device-tested working on iOS in this very run** |
| Tests | every one of them hand-wrote the tag list; none asked whether the scanner could *produce* it |

The first real symptom would have been a sideloaded or installed build reporting `denied`
against a permission the app never asked for.

Note how narrow the broken path is, and how wide the evidence *against* a bug looked. The
feature was verified working on an iOS device **and** on an Android emulator, by hand, by
someone deliberately hunting a microphone problem — and it was then "fixed" twice on Android,
with a logcat trace and a `dumpsys package` confirmation behind the diagnosis. All of that is
real evidence. None of it touched a `goleo build android` artifact: iOS hardcodes the string,
`emulate` reads a different manifest, and the `dumpsys` output came from an `emulate` install,
so the `RECORD_AUDIO` it showed came from the static file rather than from the derived one
under investigation.

**"I tested it and it worked" bounds a defect; it does not disprove one.** The question is
which artifact the test built — and here the two most-exercised paths on the platform were
both paths that bypass the thing that was broken.

**The transferable point, and it is the third time this shape has appeared here** (dialogs
with no shell registration, `IOSUsageDescs` with no reader, now a registry entry with no
scanner pattern): a declaration is only load-bearing if something *consumes* it, and two
lists edited independently drift silently. `TestEveryFeatureIsDetectable` now requires every
`featureRegistry` entry to be named by a Go scan pattern, and
`TestEveryScanPatternNamesARealFeature` requires the reverse — a pattern naming a feature the
registry lacks matches text and discards it.

### The second warning was real too

```
warning: All interface orientations must be supported unless the app requires full screen.
```

XcodeGen's application preset sets `TARGETED_DEVICE_FAMILY = "1,2"`, so **every** generated
project is iPad-capable whether or not anyone asked for it, and an iPad app that does not set
`UIRequiresFullScreen` can be handed any orientation in Split View. The plist declared three.
Fixed by adding `UISupportedInterfaceOrientations~ipad` with all four rather than by
requiring full screen — opting out of multitasking is the worse trade for a WebView UI that
reflows anyway. The iPhone list still omits upside-down, which is the convention.

### The runtime log's one substantive line, now acted on

```
`UIScene` lifecycle will soon be required. Failure to adopt will result in an assert in the future.
```

`AppDelegate.swift` created its `UIWindow` in `didFinishLaunchingWithOptions` with no
`UIApplicationSceneManifest`. Nothing was broken — this is a migration ahead of the assert,
and `GoleoUI` already existed precisely because the *partial* scene world broke
`UIApplication.shared.windows`. **Unverified on hardware at the time of writing**: no test on
a non-Mac host can run a launch path.

The split is per-app versus per-scene, and only the window is per-scene. Starting the Go
server, registering providers, the `BGTaskScheduler` registration (which *must* complete
before `didFinishLaunching` returns) and the notification delegate all stay in `AppDelegate`.
The WebView is still built in `didFinishLaunching` and merely attached when the scene
connects, so the server-start / page-load ordering that was device-verified is unchanged.

Three things that are not obvious, each of which fails as a **black screen with no build
error**:

- `UISceneDelegateClassName` is a **string** naming a Swift class, so it needs the module
  prefix: `$(PRODUCT_MODULE_NAME).SceneDelegate`. That resolves only because
  `ProcessInfoPlistFile` runs with `-expandbuildsettings` and `xcodegen.yml` pins
  `PRODUCT_NAME`. `AppDelegate` therefore *also* sets `configuration.delegateClass` in
  `configurationForConnecting` — the plist is the declaration, the code is the guarantee, and
  `TestSceneLifecycleIsAdoptedInBothFiles` requires both plus the class itself.
- A deployment target below iOS 13 compiles and signs perfectly and then ignores the scene
  manifest, so nothing creates a window. `validIOSVersion` now refuses anything below 13 and
  says why. The alternative — a `#available` fallback in the shell — is dead code that Swift
  warns about at the 15.0 default, i.e. it would trade a fixed warning for a new one.
- `applicationWillTerminate` is **not** guaranteed under scenes (a suspended app is killed
  without it). `GomobileStopServer()` stays there anyway rather than moving to
  `sceneDidDisconnect`: the process is going away with the server inside it, whereas a scene
  disconnect can happen to an app that keeps running, and stopping the backend under a live
  app is a real failure rather than a skipped cleanup.

### Ruled out (device runtime log), so nobody re-investigates

| Line | Verdict |
|---|---|
| `ios-check starting up...` printed **twice** | not a double start. It is `log.Println` → stderr, which gomobile duplicates on iOS; the `fmt.Printf` port banner next to it appears **once**, and a second `StartServer` would have fallen forward to 9843 and printed a second banner |
| `Could not create a sandbox extension for .../GoleoApp.app` | normal for a development-signed install |
| `(Fig) signalled err=-12710` ×3 | CoreMedia internal logging around capture setup; no failure followed it |
| `xpc_user_sessions_get_foreground_uid() failed`, `DeviceIdHashSaltStorage`, `Unable to hide query parameters` | WebKit/system noise present in any WKWebView app |

### Process note: `(added in X)` markers drift at release time

`docs/agents/host-features.md`'s microphone heading said 0.10.9 when the feature landed and
said **0.10.13** by the time this run happened — bumped by four consecutive release commits,
each rewriting it to the version being cut. The Dialogs heading two sections below has said
0.10.7 throughout, so nothing automated is doing it: the marker reads as "current version" to
whoever prepares a release, and a "version" in a docs diff looks like something to bump.

It matters because that marker is how anyone later works out which published release first
contained a feature — and it is the number that ends up in a MIGRATING entry. `git describe
--contains <commit>` is the authority; the heading now carries a comment saying so.

### CI now fails on unreachable CLI code (2026-08-11)

Go does not warn about a function nobody calls, and the CLI had accumulated two:
`buildAndroidDev()` (120 lines, above) and `featureForTag()`. Neither had ever had a caller.
`go build`, `go vet`, the full test suite and the CI matrix were all green with both present.

`golang.org/x/tools/cmd/deadcode` finds them in about ten seconds, and the CI step is worth
describing because the SCOPE is what makes the output usable:

- **Rooted at `./cli/goleo`**, the binary's `main`. "Dead" then means the shipped CLI cannot
  reach it, which is the question worth asking.
- **`-filter github.com/daforester/goleo/cli`.** Without it the run also reports
  `runtime/updater`, correctly — no `main` in this repo calls it, because it is a LIBRARY
  whose exported API exists for user apps. Rooting a library reports its whole public
  surface, so `runtime/` is deliberately not covered by this check. There is no cheap fix:
  the only honest root would be a real consuming app.
- **`-test` is NOT passed**, so test files are not roots. A non-test helper used only by
  tests is therefore reported. That is intended — the CLI has none today, and one appearing
  is worth a look.
- **It exits 0 whether or not it finds anything**, like `gofmt -l`; the step fails on
  non-empty output. Pinned to `v0.48.0` rather than `@latest`, so a release that changes what
  it reports is a deliberate bump instead of a red build one morning.

Mutation-checked both ways before committing: with a one-line unreachable function added it
reports `cli\cmd\scan.go:351:6: unreachable func: zzTmpUnreachable`; with it removed the
output is empty.

**The reason this earns a CI slot rather than a cleanup commit:** `buildAndroidDev` did real
damage without ever executing. Its existence next to `buildForAndroid` implied a
dev-vs-release Android build split that does not exist, and that model is what both
ineffective microphone permission fixes were made under. Dead code is not inert when it is
also documentation.

### Two scan patterns that could never match, and the scaffold's stale iOS claims (2026-08-11)

Follow-ups from the same sweep, both found by asking "does anything actually consume this?"
of things that looked settled.

**`scanPatterns` had two `Source: "ts"` entries** — `@goleo/bridge/<feature>` imports and
`on('goleo:<feature>'` listeners — and neither could ever fire, because `detectFeatureUsage`
skips any directory named `frontend`, which is where every `.vue`/`.ts` file in a goleo
project lives. Demonstrated before deleting, with one file containing both patterns:

| Location | Detected |
|---|---|
| `frontend/src/Demo.vue` | `[]` |
| the identical file at the project root | `[goleo_camera goleo_nfc]` |

Harmless in effect — a feature is unusable until its Go-side `Register*` call exists, since
that call is what installs the `goleo:*` command the frontend invokes, so the Go patterns
already covered every real project. Deleted anyway, along with the now-single-valued `Source`
field and the `.ts/.vue/.js` branch of the walk: the danger was not wrong output, it was that
the scanner *looked* like it read the frontend. Someone would eventually rely on that.

**The scaffold told every new project that iOS was unverified.** `app.go.tmpl` said
"best-effort, unverified iOS — no Xcode available to test it", which two hardware runs have
made false. It was also wrong the other way for NFC and BLE, described as having a native
Android provider and a "best-effort, unverified" iOS one — iOS registers **no** provider for
either (`AppDelegate.swift` wires nine; neither is among them) and WKWebView implements
neither Web NFC nor Web Bluetooth, so on iOS those calls do not degrade to a fallback, they
report no provider. The demo's own `registry.ts` has said `ios:'no'` for both all along, so
the scaffold contradicted the app it ships with. Both scaffolds now say Android-only, and the
minimal one gained the `RegisterMicrophone` line it had been missing since 0.10.9.

The pattern across all four items in this session: **the repo's declarations were right and
its wiring was not.** `featureRegistry` had Microphone, `registry.ts` had `ios:'no'`,
`IOSUsageDescs` has the purpose strings. What was missing each time was something that reads
them.

### Closing the deadcode blind spot: U1000 over runtime/ too (2026-08-11)

> **Superseded in part (2026-08-12).** The reasoning below about *what* each tool measures
> still holds, and the five findings were real. What it got wrong is scope: both tools
> analyse a **single GOOS**, so the gates as described here delete live code on a
> cross-platform CLI. One of the five findings in the table — `deeplink.slug()` — was **not
> dead**, and acting on it broke the Linux build. See "both 'unused code' gates analysed one
> GOOS" at the end of this file for what happened and what the steps do now.

The `deadcode` gate added earlier is rooted at the `goleo` binary's `main` and filtered to
`cli/`, because `runtime/` is a library with no entry point to root a reachability analysis
at. That exemption was documented, and then measured: `staticcheck -checks U1000` needs no
entry point — it reports **unexported** identifiers nothing references, which is precisely
the blind spot, and it sees fields and vars as well as funcs (deadcode reports only funcs).

Five findings, and they were not all clutter:

| Finding | Verdict |
|---|---|
| `runtime/fs_scope.go` `dataOnce sync.Once` | **the bad one** — see below |
| `runtime/notify/notify_windows.go` `syscall.StringToUTF16` (SA1019, found in the same sweep) | real: **panics** on app-supplied text containing a NUL |
| `cli/cmd/emulate.go` `emulateTarget` | dead; declared beside two real flag vars, bound to no flag, read by nothing |
| `runtime/deeplink` `slug()` | **WRONG — not dead.** Called by `deeplink_linux.go`, which the Windows-host run excluded. Deleting it broke the Linux build; see the 2026-08-12 entry |
| `runtime/app.go` `mu`, `running` | vestigial, NOT a missing lock — `Quit()` is idempotent because `context.CancelFunc` is |

**`dataOnce` is why this check earns its runtime.** It sat directly above a comment
explaining why once-semantics are wrong in that exact spot: `os.UserConfigDir` legitimately
fails on mobile before `SetHomeDir` runs, so the resolution must be RETRIED, and the code
memoises under the existing RWMutex with a `cached != ""` test precisely so a failure is not
latched. A `sync.Once` named `dataOnce` next to that is an invitation to "simplify" the mutex
away and leave the filesystem scope with no root for the life of the process — a security
control failing open, from a cleanup that looks like an improvement. **A dead field that
contradicts a nearby comment is worse than dead code; it is a wrong answer left lying next to
the right one.**

The notify one is the other kind of value: `StringToUTF16` is deprecated for a reason easy to
read past — it panics on an embedded NUL where `UTF16FromString` returns an error. Title and
body come straight from `goleo:notify`, so they are app data. Now strips NULs and encodes
what remains, with `notify_windows_test.go` covering it (Windows-only, so it is a
developer-machine guard: CI cross-compiles for Windows but runs its tests on Linux).

**Only U1000 is enabled**, deliberately. The default set also raises 19 `ST1005`s (error
strings capitalised or punctuated — a house-style rule this repo does not follow) and one
`SA4000` false positive on `singleinstance_test`'s determinism check, which compares a pure
function against itself on purpose. Turning the suite on wholesale would mean twenty papercut
edits or a wall of ignore directives, and a check nobody reads is a check nobody runs.

Mutation-checked like the others: an added `func zzTmpUnused()` in `runtime/deeplink` makes
the step exit 1; removed, it exits 0. Unlike `deadcode`, staticcheck exits non-zero on
findings, so the step needs no output test.

## Cross-cutting — both "unused code" gates analysed one GOOS, and each deleted live code (2026-08-12)

The two CI gates added the day before (`deadcode` over `cli/`, `staticcheck -checks U1000`
over `runtime/` + `cli/`) were sound in what they measured and wrong in the scope they
measured it over. **Both tools analyse a single GOOS per invocation.** On a cross-platform
CLI where a helper is routinely defined untagged and called only from a `_linux.go` or
`_windows.go` file, that makes every result platform-relative — and a "this is unreferenced"
answer is only actionable if the run could see all the references.

It cost live code in **both** directions before anyone noticed, which is the part worth
keeping.

### Direction 1 — a Windows host deleted Linux code

The "delete unreachable code" sweep that shipped in 0.11.0 (its commit is not addressable —
the history was flattened on 2026-08-12, see the note at the top of this file) removed
`deeplink.slug()`. Its only caller is `platformRegister` in
`deeplink_linux.go`, behind `//go:build linux && !android`. The sweep ran on Windows, where
that file is not part of the package, so U1000 answered the question it was asked, correctly.

```
CGO_ENABLED=0 GOOS=linux go build ./runtime/...
runtime/deeplink/deeplink_linux.go:20:17: undefined: slug
```

That is a CI step (`Desktop build (cgo-free, default glaze backend)`), so the branch was red
before it ever reached the new gates. Three things hid it on the authoring machine:

| | Why it looked fine |
|---|---|
| `go build ./...` | host GOOS only — the Linux file is never compiled |
| the full test suite | same; `go test ./runtime/...` passed with `slug` deleted |
| grepping the name | `runtime/autostart` has its **own** identical `slug()`, which *is* test-referenced, so the identifier resolves everywhere you would think to look |

The third is why the deletion looked safe: two packages carry the same small helper, one
protected by a test and one not, and the unprotected one is the one whose caller is
build-tagged.

**The fix is the protection autostart already had.** `slug` is restored to `deeplink.go` and
`deeplink_test.go` now exercises it, deliberately **untagged** — an untagged test references
the helper on every platform, so no single-GOOS sweep can report it. That is the general
remedy for this shape and it costs three lines.

### Direction 2 — CI's ubuntu runner would have called live Windows code dead

Run as CI actually runs it, the same gate reports:

```
cli/cmd/android_deps.go:682:6: unreachable func: windowsSdkToolCmdLine
cli/cmd/android_deps.go:690:6: unreachable func: winQuote
```

Both are live: `sdkmanager_windows.go` calls `windowsSdkToolCmdLine` to build the `cmd /s /c`
line that keeps `platforms;android-34` from being split on the semicolon. They sit untagged
**on purpose** — the comment above them says "defined here (untagged) so it is unit-testable
on any platform" — and `android_deps_test.go` tests both.

So the gate would have failed on its first CI run and asked the next person to delete working
Windows code, having already caused the deletion of working Linux code. The gate's own
mutation check did not catch this because it added a function that was dead on *every*
platform, which is the easy case. (It added it to `runtime/deeplink` — the very package where
`slug` was about to break.)

### What both steps do now

Run the tool once per desktop GOOS and fail only on a finding present in **all** of them:
"no platform we ship can reach it", which is what the gate always meant to say.

- `buildAndroidDev` and `featureForTag` — the finds that motivated the gate — were untagged
  and dead everywhere, so they are still caught. Mutation-verified: an added unreachable func
  and an unused unexported var appear in all three runs and fail both steps; removed, both
  pass.
- **Accepted gap, in the step comment so nobody rediscovers it as a bug:** something defined
  inside a single platform's build tags and dead *there* is absent from the other two runs,
  so the intersection misses it. A false negative there is cheaper than a red build that
  pressures someone into deleting live code — which is not hypothetical, it is both halves of
  this entry.

### The mechanical trap in running a Go analyser for another GOOS

`go run tool@version` with `GOOS` set cross-builds **the tool** and then cannot execute it:

```
GOOS=linux go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 -checks U1000 ./...
exec: ".../b001/exe/staticcheck": executable file not found in %PATH%
```

`go install` the tool for the host **once**, then set `GOOS` only on the invocation. The env
var has to reach the analysis without reaching the build of the analyser.

### The transferable point

The 2026-08-11 entry ended on "a declaration is only load-bearing if something consumes it".
This is the adjacent failure: **a tool that reports what nothing consumes is only as honest
as the set of files it compiled.** Build tags make that set a variable, and neither tool says
which GOOS it assumed. Any "X is unused" result on this repo is incomplete until it names the
platform it holds for — and a `deadcode`/U1000 finding on a helper next to a `_windows.go` or
`_linux.go` sibling should be assumed a false positive until checked on that platform.

## iOS — the floor was the version that BUILDS, not the version that works (2026-08-12)

`validIOSVersion` was given a floor of 13.0 the day before, because below 13 the
`UIApplicationSceneManifest` is ignored, no window is created, and the app launches to a black
screen. That reasoning was right and the number was wrong: **13.0 is where UIScene starts
working, which says nothing about whether the rest of the shell does.**

Prompted by a one-line instruction — target versions the app fully works on rather than ones
it merely builds on — which turns out to be a different question with a different answer.

### What actually binds the floor

`AppDelegate.swift` registers **nine** providers. Camera and geolocation are **not** among
them. So on iOS those two features reach the hardware only through the WebView's web APIs, and
WebKit denies both unless the app answers the matching `WKUIDelegate` callback:

| Callback | `@available` | Feature it gates |
|---|---|---|
| `requestMediaCapturePermissionFor` | **iOS 15.0** | camera + microphone via `getUserMedia` |
| `requestGeolocationPermissionFor` | **iOS 15.4** | `navigator.geolocation` |

Both are annotations on **declarations**, not `if #available` branches — so a lower deployment
target compiles cleanly, signs, installs, launches, draws, and iOS simply never calls the
method. There is no error at any stage.

The demo's own `registry.ts` claims `ios: 'yes'` for camera and geolocation. Below 15.4 that
claim was false, which makes this the **fourth** instance of this repo's recurring shape: a
declaration whose consumer does not exist. `featureRegistry` had Microphone with no scanner
pattern; `IOSUsageDescs` had strings with no reader; `registry.ts` said `ios:'no'` for NFC/BLE
while the scaffold said otherwise; and now `registry.ts` said `ios:'yes'` for two features the
deployment target could silently disable.

### The floor and the default both moved

Floor **13.0 → 15.4**, and the default **15.0 → 15.4**. The default mattering is the part worth
noting: it was 15.0, so *a project that configured nothing at all* had non-working geolocation
on 15.0–15.3. The refusal alone would not have caught that, because nothing was being refused —
the default was the broken value.

`14.0` also appears in the shell (`UNNotificationPresentationOptions.banner`) but does **not**
constrain the floor: it is an `if #available` with a real `.alert` fallback. That is the
distinction the check has to encode — a runtime branch degrades, an annotated declaration
disappears.

### The guard, because two lists edited independently always drift

`TestNoShellDelegateNeedsMoreThanTheIOSFloor` parses every `@available(iOS X.Y, *)` in
`AppDelegate.swift` and fails if any exceeds `iosFloorMajor.iosFloorMinor`. Deliberately
matches `@available` only, never `#available`. Mutation-checked: with the floor set back to
15.0 it fails and names the offender —

```
AppDelegate.swift declares @available(iOS 15.4, *), above goleo's floor of 15.0.
```

It also asserts `validIOSVersion(defaultIOSDeployTarget)` passes, so the default can never be a
value the validator refuses — which would make `goleo build ios` refuse every project that did
not override it.

Adding an iOS-17-only delegate is now a deliberate fork: raise the floor, or write a fallback.
Neither happens by accident.

### Cost, stated rather than hidden

15.0–15.3 are no longer supported targets. That window is genuinely narrow — 15.4 shipped March
2022 and 15.x devices still receiving updates sit on 15.8 — and the alternative was shipping an
app whose camera and location pages fail with no diagnosis on a target we advertised as
supported. There is deliberately **no override flag**: lowering the constant would not make the
callbacks fire, so a flag would only move the silent breakage somewhere less visible.

### The transferable point

"Refuse what does not build" is the easy half and the toolchain mostly does it for you. **The
floor worth enforcing is the one below which something is missing rather than broken** —
`@available` on a delegate is the canonical shape, because the compiler is *satisfied* and the
platform just never calls you. When a minimum version is chosen, the question is not "does it
compile" but "which of the features we advertise stop existing", and the answer is in the
availability annotations, not in the build log.

## Geolocation — deleted the Go implementation; it was never the real path (2026-08-12)

`runtime/geolocation/` is gone. Geolocation is now `navigator.geolocation` in the webview on
every platform, which is what **five of the six platforms already did**.

### What the "native" tier actually was

| Platform | Implementation | Reached a real user? |
|---|---|---|
| Windows | WinRT `Geolocator` driven through a **PowerShell 5.1 subprocess per call** (the WinRT projection does not exist in pwsh 7+) | yes — a process launch to fetch a coordinate |
| macOS | shelled out to **`CoreLocationCLI`**, *if* `brew install corelocationcli` happened to be present | essentially never |
| Linux | `ErrUnsupported` — would have needed a GeoClue D-Bus client | no |
| Android | no provider registered by the shell | no — already the webview |
| iOS | no provider registered by the shell | no — already the webview |

So the Go side was one subprocess on one platform, an optional Homebrew dependency on another,
and nothing on the remaining four. The tell was already in the tree: Linux's WebKitGTK
permission auto-grant (`webview_glaze_permissions_linux.go`) exists, in its own words, "so the
app's getUserMedia/**geolocation** fallbacks resolve instead of hanging". **The browser path was
the design in practice long before it was the design on purpose.**

The reason it was ever written in Go is the cgo-free invariant: `geolocation_darwin.go` says it
outright — "CoreLocation needs an Objective-C delegate, which pure Go cannot provide without
cgo". Both desktop implementations were workarounds for that constraint, and the webview does
not have it. WebKit and WebView2 call CoreLocation and the WinRT Geolocator themselves, with the
real permission UI and no process launch.

### The trap: deleting the feature would have broken the web path

`RegisterGeolocation` **stays**, and it now installs no handler — an empty exported function,
which is exactly the shape a cleanup deletes. It cannot be deleted, and the reason is not local
to the file:

- `detectFeatureUsage` scans for `RegisterGeolocation(` and emits `goleo_geolocation`
- that tag is what `resolveAndroidPermissions` turns into `ACCESS_FINE_LOCATION` +
  `android.hardware.location*`, and what `featureRegistry` maps to
  `NSLocationWhenInUseUsageDescription`
- **Android's WebView can only grant a `navigator.geolocation` request if the app itself holds
  `ACCESS_FINE_LOCATION`**, and WKWebView needs the usage description

So the permission declaration is not an artifact of the old native path — it is a prerequisite
of the *new* web one. Removing "the Go implementation" in the obvious way (delete the package,
delete the registration) would have produced an app that compiles, ships, claims
`android:'yes'`, and fails when a user taps the button.

`TestGeolocationStaysDetectableAsAPureWebFeature` asserts all four links: the scanner still
emits the tag, the tag still derives `ACCESS_FINE_LOCATION`, the registry still carries the iOS
usage description, and `KnownCommands` contains **no** geolocation method (a typed `invoke()`
overload for an unhandled method is a call that always fails). Mutation-checked on two of them:
restoring the schema entry fails assertion 4, emptying the registry permissions fails
assertion 2.

This is the **fifth** appearance of the repo's recurring shape — a declaration whose consumer
is not obvious — and the first where the declaration is a *deliberately empty function*. Worth
naming the variant: an empty body is not evidence of dead code when the call site itself is the
signal being consumed. `grep` for the identifier, not for what the body does.

### Process note: `git checkout` during mutation testing ate an unstaged edit

Mutating `schema.go`, testing, then `git checkout cli/cmd/schema.go` reverted the mutation **and
the real change underneath it**, because the real change had not been staged. The follow-up test
run went red for a reason that looked like the fix failing. `git add` before mutation-testing, or
the revert is not scoped to the mutation.

### What is deliberately lost

`navigator.geolocation` only runs while a page is alive and foregrounded, so background
location — significant-change monitoring, geofencing — is unreachable. It was unreachable
before too (no shell ever registered a provider), so nothing regressed. That is the one
capability that would justify a real native provider in the mobile shells, and it is the only
argument for one: the earlier plan to write a `CLLocationManager` provider to lower the iOS
floor from 15.4 to 15.0 is **dropped**, not deferred, because it would have built the first
native mobile provider for a feature being consolidated onto the web path, to recover a version
window that excludes no device (every iPhone that can run iOS 15 was offered 15.8.x).

### Companion: the shell-out inventory

Doing this raised the obvious question of what *else* reaches the OS by launching a process, so
`docs/agents/external-binaries.md` now catalogues it — split into **runtime** binaries
(dependencies of the shipped app: `xclip`, `zenity`, `osascript`, `pmset`, `caffeinate`,
`systemd-inhibit`, `notify-send`, `xdg-open`, `rundll32`, `powershell`) and **CLI** binaries
(dependencies of the build machine). The runtime half is the one worth knowing: a feature can be
absent at runtime on a machine that compiled it fine, CI is headless Linux where several are
missing so those tests *skip* rather than fail, and green CI is therefore not evidence any of
it works.

## Build sweep — every target exercised against a real scaffold (2026-08-12)

Ran the whole `goleo build` matrix against both scaffolds using the **published** v0.11.0
module, rather than `go build ./...`. Twelve paths, all correct, and three defects that only
running them could find.

### What passed, and what the artifacts actually were

| Target | Artifact | Verified as |
|---|---|---|
| current / windows | `app.exe` | **PE32+ x86-64** (`file`, not just exit 0) |
| darwin | `app` | **Mach-O 64-bit x86_64** |
| linux | `app` | linux/amd64 |
| pwa | `dist-pwa/` | `index.html` + `manifest.json` + `sw.js` |
| android (demo) | `app.apk` 67 MB | `BUILD SUCCESSFUL` |
| android (minimal) | `app.apk` 66 MB | see below — this is the one that matters |
| android --release --no-sign | `app.aab` 50 MB | unsigned |
| android --release (signed) | `app.aab` 50 MB | `META-INF/UPLOAD.RSA` present |
| --bundle | `demoapp-0.1.0-setup.exe` 6 MB | NSIS |
| ios / ios --simulator | — | refused cleanly on Windows, with guidance |
| --release without a key, --publish without a key | — | refused cleanly |

**The microphone fix, proven in the shipped artifact** (`aapt2 dump` on the APK, not the
template): `RECORD_AUDIO` and `MODIFY_AUDIO_SETTINGS` present, every hardware feature
`uses-feature-not-required`, only `faketouch` implied-required. `ACCESS_FINE_LOCATION` is
there too — with geolocation now having **no Go implementation at all**, which is the
declaration-only `RegisterGeolocation` path working end to end.

### Testing only the demo scaffold would have missed the point

The demo enables every feature, so it cannot catch the `nativeShellProviderTags` class of
bug. The **minimal** scaffold — what `goleo new` produces by default, with every `Register*`
commented out — is the case where the fixed Java/Swift shell references providers the app
never enabled. It builds clean.

And permissions genuinely follow enablement, measured rather than assumed:

| | goleo derived | in the APK |
|---|---|---|
| demo (registers everything) | 16 | 20 |
| minimal (registers nothing) | 3 | 7 |

`CAMERA`, `RECORD_AUDIO`, `ACCESS_FINE_LOCATION`, `BODY_SENSORS` and `NFC` are all **absent**
from the minimal APK.

### The gap in that table: the manifest merger adds permissions goleo never declares

Both counts are higher in the APK than in the derived manifest, and the difference is not
goleo's. For the minimal app, goleo declares exactly three (`INTERNET`,
`ACCESS_NETWORK_STATE`, `POST_NOTIFICATIONS`) and Gradle's manifest merger injects four more
from library manifests:

```
WAKE_LOCK   RECEIVE_BOOT_COMPLETED   FOREGROUND_SERVICE   <pkg>.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION
```

So **an app that registers nothing still ships `WAKE_LOCK` and `RECEIVE_BOOT_COMPLETED`**,
both of which are visible on a Play listing. This is the same shape as the implied
`<uses-feature>` problem already documented in `android_permissions.go` — a permission
arriving from somewhere other than the `Register*` call — and the build's "Detected mobile
features" report cannot show it, because it reports goleo's derivation and the merge happens
later, inside Gradle.

Not changed here, deliberately: suppressing them needs `tools:node="remove"`, and `WAKE_LOCK`
is legitimately required the moment `RegisterWakeLock` is used, so a blanket removal would
break the feature it belongs to. Recorded so the next Play upload is not a surprise —
**check the listing's permission list, not the build output.**

### Three CLI defects, none reachable by reading the code

1. **Every failing command printed its error twice.** `SilenceUsage` was set on `rootCmd`,
   `SilenceErrors` was not, so cobra printed `Error: <msg>` and `Execute()` printed `<msg>`
   again. Invisible on a one-line error; the keystore message is nine lines and came out as
   eighteen, reading like two different failures.
2. **The keystore hint could not be pasted.** The Go source had `"\n"` where it meant a
   shell line-continuation plus a newline (`"\\n"`), so the printed `keytool` command
   carried a literal `\n` mid-line.
3. **It recommended a tool that is not there.** It said to run `keytool` — confirmed **not on
   PATH** on this Windows machine — while `goleo generate android-key` exists precisely for
   that and uses the JDK goleo already resolves. Verified working: it produced `release.jks`
   with no `keytool` on PATH, and that key then signed a real `.aab`.

The transferable bit: (1) and (3) are both *only* observable by running the failure path.
Nothing asserts stderr shape, and no test reads a help string, so a message can rot
indefinitely. The keystore text is the one a user meets at exactly the moment they are
trying to ship.
