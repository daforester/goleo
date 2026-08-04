# SPIKES.md — Feasibility findings

Durable record of the de-risking spikes run for the desktop / cgo-free / in-process
architecture. These results are the evidence behind the decisions in
[`docs/roadmap.md`](docs/roadmap.md). **Don't re-run these from scratch — read here first.**

Dates are when verified. Environment: Windows 11 host, Go 1.26, Docker (Linux), GitHub Actions
(macOS), an Android emulator.

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
