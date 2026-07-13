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
