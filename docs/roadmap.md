# Goleo Masterplan

> The single authoritative plan for Goleo's evolution. Consolidates the former
> desktop-parity roadmap and the device-feature plan (`PLANS.md`, now absorbed).
> Two tracks run in parallel:
> - **Track D — Desktop parity & architecture** (windowing, lifecycle, distribution, security, transport).
> - **Track F — Device features** (Capacitor-style host features on desktop + mobile).
>
> Cold-start orientation: read [`AGENTS.md`](../AGENTS.md), then this file.

---

## Completion status (2026-07-10)

**The framework is feature-complete and shipping-ready on every platform via the
implemented paths.** Verified green: all runtime test packages, host + windows(cgo-free) +
android + ios + mirror builds, tsc.

Done and committed: multi-window (multi-process everywhere + in-process on Windows),
capability guards + runtime ACL (D3a), server hardening (D3b), KV store (D2), Share + clipboard
device features (Android native, iOS blind), the full distribution loop (bundle → sign →
publish → updater, D1), and the complete desktop lifecycle/OS-integration set (signal-based
Quit, ExitOnClose, single-instance, autostart, tray + Background/headless mode, deep-link/URL
scheme). Android is runtime-verified on an emulator; Windows multi-window on the dev's desktop.

**DONE (2026-07-13): cgo-free in-process webview on macOS/Linux + native-bind transport.** Adopted
`crgimenes/glaze` (WKWebView + WebKitGTK + WebView2 on `ebitengine/purego`) as the default
macOS/Linux backend, so **every desktop target is now cgo-free and cross-compiles from one machine**
(`CGO_ENABLED=0`, `runtime/cgo` absent). Native IPC (`Config.NativeIPC`) and in-process multi-window
(`mainLoopWindowManager`, `Config.InProcessWindows`) both ship. **Verified on real hardware, all
three OSes:** Windows (WebView2), Linux/WebKitGTK (Docker+WSL & `glaze-verify.yml` ubuntu), and
macOS/WKWebView (`macos-14`). The legacy cgo `webview_go` and Windows `go-webview2` backends have
both since been removed — glaze is the sole desktop webview, no cgo webview path. The system tray
works on all three desktops (macOS via a purego/objc
`NSStatusItem` backend that shares glaze's fakecgo — `tray_darwin.go`). **Native menu bar**
(`Config.Menu`/`App.SetMenu`, `runtime/menu.go`) ships on **all three desktops**, all cgo-free via
purego: macOS (objc `NSMenu`), Windows (user32 + wndproc subclass), Linux **GTK3** (`GtkMenuBar` +
accelerators) **and GTK4** (GMenu + `GtkPopoverMenuBar`). Plus a **bridge menu API**
(`goleo:setMenu` + `@goleo/bridge` `setMenu`/`onMenu`) for frontend-defined menus (leaf items emit
`menu:<id>` events). Verified: Windows (local GUI), Linux GTK3 + GTK4 (Docker), macOS (`macos-14`).
Residual caveats: accelerators are full on macOS/GTK3, best-effort on Windows/GTK4;
interactive/pixel UX only headless on CI. See Track D, `SPIKES.md`, and `spikes/glaze-*`.

> **Currency note (2026-08-03).** Entries below that describe `goleo://` asset serving as
> *deferred*, or the custom-scheme API as *requiring a glaze fork*, are **history**. `goleo://`
> shipped as opt-in `Config.SchemeAssets` on all three desktops, and the scheme API has since been
> **merged and released upstream** in `crgimenes/glaze v0.0.46`. The `daforester/glaze` fork is
> still pinned, but now solely for the Windows WebView2 permission auto-grant; it is a rebase of
> upstream `v0.0.46` plus that one commit (`v0.0.46-goleo.1`). See `SPIKES.md` (2026-08-03).

## 0. Current status (what is built vs designed)

**Built & verified (uncommitted WIP unless noted):**
- **cgo-free webview on all three desktops** — the `glaze` binding (`runtime/webview_glaze.go`;
  WKWebView / WebKitGTK / WebView2 via `purego`). `CGO_ENABLED=0` builds and cross-compiles for
  every desktop target. (Windows originally used `jchv/go-webview2`; unified onto glaze 2026-07-14,
  and the dep + `runtime/webview_windows.go` were removed — later `go-webview2` references in this
  doc are historical.)
- **Multi-window (interim, multi-process)** — `runtime/windowmanager.go` + `window_child.go`:
  each extra window is a child process hosting one webview against the shared loopback server.
  `App.OpenWindow/CloseWindow/ListWindows`, bridge `goleo:window*`, `bridge/src/window.ts`.
- **Capability guards** — `runtime/capabilities*.go`: `WindowingSupported()`/`TraySupported()`,
  `errors.ErrUnsupported`-wrapped guards on the desktop APIs, `goleo:capabilities` query,
  TS support checks. Desktop APIs refuse gracefully on mobile/PWA.
- **Docs** — `AGENTS.md` updated (dual webview backend + multi-window); this masterplan.
- **D3b server hardening** — loopback bind + Origin allow-list + per-launch token (see §2).
- **Share feature (Track F, desktop-complete)** — `runtime/share/*` (native URL hand-off on
  Win/mac/Linux, mobile provider interface, stub), `runtime/share_reexport.go`,
  `bridge/src/share.ts` (Web Share API + clipboard fallback), `scan.go` + `schema.go`
  registered (`goleo:share`, tag `goleo_share`). **Remaining for full mobile:** gomobile
  provider template (`tmplMobileShareGo`), Android/iOS shell wiring, a `ShareDemo.vue`, the
  `create-goleo-app` template mirror, and dist rebuild — all need an emulator to verify.
- **Share sheet — Android provider wired (complete)** — `tmplMobileShareGo` + generator entry
  + `GoleoShare` (`Intent.ACTION_SEND`, UI-thread) in both android shells; `RegisterShare`
  added to the scaffold `app.go`. Android verified to compile; run to confirm. **Remaining:** a
  `ShareDemo.vue` demo page (optional).
- **iOS providers wired blind (UNVERIFIED)** — `GoleoClipboardImpl` (`UIPasteboard`) +
  `GoleoShareImpl` (`UIActivityViewController`) added to `AppDelegate.swift` + registered. No
  Xcode/device here, so gomobile's exact Swift protocol signatures/arg-labels are a best guess
  (marked with NOTE comments) — needs a Mac to validate.
  - **Currency note (2026-08-05):** validated. `AppDelegate.swift` now compiles and the app
    runs on a simulator in CI. The guessed *method shapes* were all correct; the **names**
    were not — the Swift module is `Goleo` (from the artifact) while every symbol carries the
    Go *package* prefix `Gomobile`, and package-level Go funcs are C functions taking no
    argument labels. Read off the generated `Gomobile.objc.h`, which `mobile-verify` prints.
    Clipboard/Share are compiled and wired but not exercised on a device; a simulator has no
    real pasteboard peer or share sheet to assert against.
  - **Currency note (2026-08-09):** exercised on an iPhone 17 Pro Max. Clipboard round-tripped
    both directions; the **share sheet did not open** — the presenter was resolved through
    `UIApplication.shared.windows`, which is empty under the scene lifecycle, so `present`
    was a silent no-op. Fixed via `GoleoUI.topViewController()`, not yet re-run on hardware.
    Full findings in `SPIKES.md` (2026-08-09).
- **npm mirror synced** — `cli/npm/goleo/` (runtime + bridge src/dist + `go.mod`) resynced
  with all recent work; mirror module verified to build on host, windows (cgo-free), and the
  android mobile guard, and the store test passes there.
- **D1c Auto-updater (client core, complete + tested)** — `runtime/updater/`: signed-manifest
  **ed25519** verification, numeric version compare, HTTP fetch + SHA256-checked download, and
  self-replace/relaunch (`ApplyAndRelaunch`). Reexport `goleo:updater{Check,Apply}` +
  `updater:progress` event; `bridge/src/updater.ts`; typed schema. Unit-tested: sign→verify
  round-trip, tamper + wrong-key rejection, version compare, check logic. Synced to the npm
  mirror. **Remaining:** self-replace/relaunch needs real-app validation; the manifest-publish
  side belongs to the bundler (**D1a**, below) + signing (**D1b**).
- **D1a Bundler (`goleo build --bundle`, plumbing complete)** — `cli/cmd/bundle.go`: per-OS
  installer packaging into `dist/bundle/`, config from `goleo.json` (`app_name`/`version`/
  `bundle`{identifier,publisher,icons}). Windows → NSIS (`makensis`, generated `.nsi`);
  macOS → `.app` bundle (**pure Go**) + `.dmg` (`hdiutil`); Linux → `.deb`/`.rpm` (`nfpm`).
  Missing tools yield a clear install-hint error, not a cryptic failure. Unit-tested: `slug`
  + generated Info.plist/NSIS/nfpm content. **Not verifiable here** (needs the packaging tools
  + target OS to emit real installers); AppImage/WiX(.msi) and `--publish` (write the signed
  updater manifest) are follow-ups. CLI-only — reaches npm users via a rebuilt `goleo` binary,
  not the runtime mirror.
- **D1b Code signing & notarization (plumbing complete)** — `cli/cmd/signing.go`, hooked into
  the bundler: Windows Authenticode (`signtool`, timestamped SHA-256 — signs app binary +
  installer), macOS `codesign` (deep, hardened runtime) + `notarytool` submit/`stapler`.
  **Env-driven** (`GOLEO_WIN_CERT[_PASSWORD]`, `GOLEO_MAC_IDENTITY`, `GOLEO_APPLE_ID`/
  `_TEAM_ID`/`_PASSWORD`) so secrets stay out of the repo and CI injects them; unset →
  signing is **skipped with a notice**, not a failure. Unit-tested: env enable/disable logic.
  Real signing needs certs + the target OS (not verifiable here). Linux package signing is a
  follow-up.
- **D1 closed — `goleo build --publish`** — writes the ed25519-signed update manifest the D1c
  client consumes, closing the loop `build → bundle → sign → publish → auto-update`. Copies the
  built binary to a platform-named artifact, SHA256s it, merges a `Release` for the current
  platform into `dist/bundle/manifest.json`, and signs with `GOLEO_UPDATE_PRIVKEY` (repeated
  per-OS runs accumulate). Added `updater.SignManifest` (counterpart to `VerifyManifest`) and
  `goleo generate updater-key` (prints an ed25519 keypair). Unit-tested: `mergeAndSign`
  round-trips through the real verifier + accumulates/overwrites platforms. Mirror synced.
  **D1 (distribution) is now coherent end-to-end**; remaining niceties: AppImage/WiX,
  Linux GPG signing, and running the real toolchain on each OS.
- **D4 kickoff — Windows in-process multi-window spike** — `spikes/win-multiwindow/`: two
  `go-webview2` windows in one process, each on its own locked OS thread (Windows gives each
  thread a message queue), distinct WebView2 data dirs. Cross-compiles cgo-free
  (`CGO_ENABLED=0 GOOS=windows`). Tests whether in-process multi-window is *cheap* on Windows
  (no `edge`-layer single-loop rewrite) — the alternative to today's multi-process model.
  **Runnable on the developer's Windows desktop** (`go run .`); PASS = two independent windows.
  Outcome decides D4.0's Windows path (multi-thread vs. hidden-master single-loop).
  **Result: ✅ PASS** (ran on the dev's Windows desktop) — see `SPIKES.md`.
- **D4.0 in-process WindowManager (Windows, opt-in)** — built on the passing spike:
  `inProcWindowManager` (`runtime/windowmanager.go`) hosts each extra window on its own
  `LockOSThread` goroutine instead of a child process; close via the webview's
  `Dispatch`+`Terminate` (new methods on `WebviewWindow`). Selected by `Config.InProcessWindows`
  on Windows (else the multi-process manager, unchanged — non-regressive) via a `windowSpawner`
  interface both implement. Compiles host/windows/android/mobile-stub; run to verify on
  Windows. macOS/Linux stay multi-process until their in-process bindings land (AppKit is
  main-thread-only — the per-thread trick is Windows-specific). Spike findings recorded in
  `SPIKES.md`.
- **D4 lifecycle backbone — signal-based Quit + per-window ExitOnClose** — `App.Quit()` is the
  single idempotent shutdown funnel (unblocks the run loop → CloseAll → OnShutdown → stop
  server); `Stop()` is now an alias. `goleo:quit` bridge command + `quitApp()` TS. Both window
  managers track `WindowOptions.ExitOnClose` and call `Quit()` when such a window closes.
  Unit-tested: Quit cancels/idempotent/no-cancel-safe, ExitOnClose plumbing, both managers
  satisfy `windowSpawner`. Cross-platform, mirror synced. **Remaining lifecycle:** the
  `Config.Background` daemon/headless-controller mode and the tray (both main-thread-coupled,
  come with the tray increment).
- **D4 single-instance (complete, cross-platform, pure Go)** — `runtime/singleinstance/`: the
  first launch binds a per-app loopback address and becomes primary; a later launch forwards
  its args (with an ACK handshake, so an unrelated program on the port isn't mistaken for us)
  and **exits**. The primary emits `app:secondInstance{args}` (for focusing a window / deep
  links). Opt-in via `Config.SingleInstance` (+ `AppID`); acquired before the server binds;
  released on shutdown. **Fully unit-tested** with real in-process loopback IPC
  (acquire/forward/ACK, re-acquire after close) — no GUI needed. Also the daemon "wake"
  mechanism and the basis for deep-linking. Cross-platform; mirror synced.
- **D4 autostart (complete)** — `runtime/autostart/`: launch-on-login via Windows HKCU Run key
  (cgo-free `x/sys/windows/registry`), macOS LaunchAgent plist, Linux `~/.config/autostart`
  .desktop; mobile/wasm → `ErrUnsupported`. `goleo:autostart{Enable,Disable,IsEnabled}` +
  `bridge/src/autostart.ts`. Unit-tested generators; darwin cross-compile verified.
- **D4 tray + Config.Background (complete)** — headless-controller mode (no auto primary
  window; main thread runs the tray or blocks until Quit) + `Config.Tray` via `gogpu/systray`
  (cgo-free) with Go `OnClick` callbacks; `Config.OnReady` (post-server hook where OpenWindow
  works). `runtime/tray_desktop.go` / `tray_stub.go` (excluded on mobile). Builds windows
  cgo-free + android-mobile-guard; run on Windows to verify UX.
- **D4 deep-link / URL scheme (complete)** — `runtime/deeplink/`: register a `myapp://` scheme
  (Windows registry, Linux `x-scheme-handler` .desktop + xdg-mime, macOS via the `.app`
  Info.plist `CFBundleURLTypes` the bundler now emits). `Config.URLScheme`; the launch URL is
  read via `goleo:initialURL`, later launches forward through single-instance → `app:openURL`
  (`bridge/src/deeplink.ts`: `getInitialURL`/`onDeepLink`). Unit-tested; cross-platform; mirror
  synced. (macOS URL *handling* still needs the native app layer.)
- **D3a Capability ACL (central enforcement, complete)** — `runtime/policy.go`: a `Policy`
  (Allow list with `prefix*` wildcards + always-safe core commands) enforced **centrally in
  `Bridge.HandleRequest`** (deny-by-default when a policy is set; no policy = legacy-permissive,
  so nothing breaks by default). `App.SetPolicy`/`Bridge.SetPolicy`. Scope helpers
  (`AllowsFSPath` with traversal-safe cleaning, `AllowsHTTPHost`, `AllowsShellProgram`) ready
  for plugins. Unit-tested: method allow/deny (exact/prefix/core), fs/http/shell scopes, and
  that a denied handler never runs. Mirror synced. **Remaining:** wire the scope checks into
  the individual plugins (fs now; http/shell when built in D2).
- **Android dev secure-context fix** — `goleo emulate android` now serves the frontend over
  **`http://localhost:<vitePort>` via `adb reverse`** instead of `http://10.0.2.2` (which is
  *not* a secure context, silently disabling the WebView's secure-context-only APIs:
  `getUserMedia`/camera, clipboard, geolocation). This makes dev match production
  (`127.0.0.1`, already secure), so those demos work in emulation. `emulate.go` (adb reverse)
  + `android-dev` `MainActivity` (loadUrl + permission-origin → `localhost`). Root-cause fix
  for the whole class of secure-context features; the clipboard native provider (below) stays
  as the more robust path.
- **Clipboard Android native provider (bug fix)** — clipboard was half-wired: the Go
  `Provider`/`SetClipboardProvider` existed, but there was no `tmplMobileClipboardGo` and no
  `GoleoClipboard` in the shells, so on Android it hit the `GOOS` default ("not supported") and
  the `navigator.clipboard` fallback fails in the WebView (insecure `10.0.2.2` in dev). Added
  the gomobile `ClipboardProvider` template + generator entry + `GoleoClipboard`
  (`ClipboardManager`, UI-thread-marshaled) in both android shells. **Remaining:** iOS
  `AppDelegate` (`UIPasteboard`), `cli/npm` mirror + dist rebuild.
- **D2 KV Store (complete)** — `runtime/store/` (JSON-file KV in the app-data dir, atomic
  writes, unit-tested; self-contained pure Go, **no build tag / permission / mobile shell** —
  works on every target incl. android/ios), `runtime/store_reexport.go` (`goleo:store*`),
  `bridge/src/store.ts` (localStorage fallback), `schema.go` typed overloads. Fully verified
  here (no emulator needed).

**Feasibility proven (spikes, see Decision Log):**
- **Windows** cgo-free build; **Linux** cgo-free `dlopen` (Spike 1); **macOS** cgo-free
  WKWebView JS↔Go round-trip on real Apple-Silicon CI (Spike 2). The cgo-free, in-process
  binding is de-risked on all three desktop OSes at the mechanism level.

**Designed, not yet built:** in-process hidden-master binding (the A2 target), tray
(`gogpu/systray`), per-window `ExitOnClose`, signal-based `Quit` + daemon lifecycle, native-bind
transport, distribution (installers/signing/updater), storage plugins, capability ACL, and the
Track-F device features.

---

## 1. Target architecture (locked)

One process. A **hidden master** owns the single native run loop and is the app's lifecycle
anchor (the controller); visible windows are created under it. Optional, developer-controlled
tray. Signal-based quit. Mobile stays on its own path, fully insulated.

- **cgo-free native webview on every desktop OS** (proven): Windows `go-webview2` (`edge`
  layer), macOS `purego`+WKWebView, Linux `purego`+WebKitGTK. As of 2026-07-12, `crgimenes/glaze`
  provides all three cgo-free behind one `WebView` interface (verified cross-compiling in
  `spikes/glaze-webview/`), so the plan is to **wrap glaze** (vendor/fork + pin) rather than port
  by hand; **Wails v3** / `webview/webview` source remain the API spec if we ever own the glue.
  - **Phase 1 DONE — glaze is the default macOS/Linux backend (`runtime/webview_glaze.go`).**
    Every desktop target now builds `CGO_ENABLED=0` with no tags (verified: windows +
    darwin/{amd64,arm64} + linux/{amd64,arm64}, `runtime/cgo`=0), so **all desktops are pure-Go
    and cross-compile from one machine**. Verified on real macOS + Linux (`glaze-verify.yml`:
    JS↔Go round-trip + WebKitGTK permission auto-grant via the purego shim). Also unblocked
    `runtime/camera` via a `cgo`/`!cgo` split. The legacy cgo `webview_go` backend
    (`runtime/webview.go`) was kept one release behind `-tags goleo_cgo_webview` as a
    fallback, then **removed** (see the status section above) — no cgo webview path remains.
  - **In-process multi-window (macOS/Linux) — DONE and verified on real hardware.** glaze does the
    single-loop master (shared `NSApplication`/GtkApplication + `windowCount`), so extra windows are
    opened by `Dispatch`-ing `glaze.New()` onto the primary's main-thread run loop.
    `runtime/windowmanager_mainloop.go` (`mainLoopWindowManager`), selected by
    `Config.InProcessWindows` on darwin/linux. The `spikes/glaze-runtime-verify` app (a real goleo
    app: native IPC + permission shim + a 2nd window via `App.OpenWindow` + clean `Quit`) **passed on
    real Linux** (Docker+WSL & `glaze-verify.yml` ubuntu) **and real macOS** (`macos-14`, Apple
    Silicon). **The cgo-free desktop stack is now verified on all three OSes** (Windows/WebView2,
    Linux/WebKitGTK, macOS/WKWebView). The tray works on all three (macOS via a purego/objc
    NSStatusItem backend). Caveat: interactive UX is only exercised headlessly on CI.
  - **`goleo://` asset serving — deferred, low priority (see `SPIKES.md`).** Native IPC already
    removed the RPC surface; the residual is a loopback-only, embedded-assets-only static server.
    The only portless option that keeps a *secure context* (localStorage/getUserMedia/routing) is a
    native scheme registered as secure — which requires **forking glaze** (macOS `WKURLSchemeHandler`
    is config-time, not externally attachable). `file://` and inline-`SetHtml` are cgo-free + portless
    but lose the secure context, so they're inadequate as a default. Sequence when pursued: scheme
    handlers inside the glaze fork → register secure → opt-in `Config`. android/ios stay cgo (gomobile).
- **In-process multi-window** under the master's run loop (Tauri/Wails model). Multi-process is
  the interim/fallback (works today with minimal bindings; the reason it can't be the end state
  is macOS dock/menu fragmentation + memory).
- **Native-bind IPC** (`go-webview2 Bind` / WKScriptMessageHandler / WebKit message handler) —
  **no loopback socket in production**. Socket retained only for **dev-mode HMR** and **mobile**.
  Custom `goleo://` scheme serves embedded assets.
  - **Shipped (opt-in, `Config.NativeIPC`):** `runtime/nativeipc.go` — a per-window `nativeSession`
    uses the webview channel (`Bind` for →Go, `Eval(window.__goleoRecv)` for →JS); the
    `@goleo/bridge` transport ladder is native → WebSocket → HTTP with transparent fallback, so
    child-process windows / browser / PWA / mobile keep the socket. Same `{type,data}` envelope +
    `Bridge.HandleRequest` (ACL applies). Covers the primary window **and in-process additional
    windows** (`InProcessWindows`). **Verified on real WebView2** (two-window round-trip + clean
    Quit) and `runtime/nativeipc_test.go`. Fixed two GUI-lifecycle bugs it exposed: the `a.ctx`
    clobber in `StartServer` (Quit hung) and the unpinned main goroutine (`Run` now
    `LockOSThread`s).
  - **Remaining — custom `goleo://` asset serving (deferred to the purego milestone):** would drop
    the loopback HTTP asset server too, not just the WS RPC surface. Deferred by decision
    (2026-07-12): the cgo `webview_go` backend exposes no scheme-handler API, and `jchv/go-webview2`
    only exposes `WebResourceRequested`/virtual-host mapping on its lower-level `edge.Chromium`
    (hidden behind the high-level `webview.WebView`), so a native scheme today would be a
    Windows-only ~200-line edge-layer rewrite. The purego mac/Linux backends are Goleo's own code
    and can add `goleo://` uniformly across all three OSes. Full finding + API pointers in
    [`SPIKES.md`](../SPIKES.md). Also then: make native IPC the default.
- **Lifecycle:** `Config.Background` (headless controller, windows optional/on-demand — daemon
  shape), optional `Config.Tray`, per-window `WindowOptions.ExitOnClose`. A single idempotent
  `Quit()` funnel (Go `App.Quit()`, JS `quitApp()`, OS signal, tray item, `ExitOnClose`) fans
  out: close tracked windows → remove tray → `OnShutdown` → stop server → exit. Orphan safety
  net via OS parent-death (Job Object / `PR_SET_PDEATHSIG`) + `app:shutdown` broadcast.
- **Capability-guarded APIs** so unsupported platforms fail with `ErrUnsupported`, never crash.

### Build model (revised — supersedes the earlier "cgo matrix" conclusion)

The spikes reversed the earlier finding. Because the bindings are **cgo-free**, builds stay
`CGO_ENABLED=0` and **cross-compilation is back on the table** (darwin was cross-built from
Windows in Spike 2). Per-OS runners are still needed for **signing, notarization, bundling, and
runtime testing** — but *not* for compilation. This is strictly better than the Tauri/Wails
per-OS cgo model.

---

## 2. Track D — Desktop parity & architecture

### D0 — remaining spikes (S)
- [ ] Linux: repeat Spike 1 against real `webkitgtk-6.0` with a `script-message-received`
      callback via `purego.NewCallback` (proves the signal/marshaling path + version choice).
- [ ] macOS: confirm the `macos-13`/amd64 job; exercise `WKURLSchemeHandler` (asset path).
- [x] Windows cgo-free build · Linux `dlopen` · macOS WKWebView round-trip — **done**.
- [ ] SQLite driver: pure-Go `modernc.org/sqlite` (avoids a second toolchain; keeps mobile/PWA clean).
- [ ] Updater: signed-manifest scheme + key custody.

### D1 — Distribution & lifecycle (L)
`goleo build` still emits a raw binary. Highest shipping value.
- **1a Bundler** `goleo build --bundle` → per-OS installers (Win `.msi`/NSIS, macOS `.dmg`,
  Linux `.deb`/`.rpm`/`.AppImage`) via wrapped tooling; new `cli/cmd/bundle.go`; config in
  `goleo.json`. Cross-compile the binaries; package on per-OS runners.
- **1b Signing/notarization** — Authenticode + `codesign`/`notarytool`, env-driven for CI.
- **1c Auto-updater** — `runtime/updater/` (vertical slice): signed manifest, download, swap,
  relaunch; `goleo:updater*` + `updater:progress`; `--publish` writes the manifest.

### D2 — Storage & core plugins (M) — standard vertical slices (§4)
| Plugin | Tag | Desktop impl | Notes |
|--------|-----|--------------|-------|
| **KV Store** | `goleo_store` | JSON/bolt in app-data (reuse `runtime/fs`) | ship first as exemplar |
| **SQL** | `goleo_sql` | pure-Go SQLite | param binding only |
| **Shell exec** | `goleo_shell` | `os/exec` | allowlist in `goleo.json`; never raw strings |
| **HTTP client** | `goleo_http` | `net/http` | host allowlist; bypasses webview CORS |
| **Log** | `goleo_log` | file + console | rotating |

### D3 — Security (M)
- **3a Capability ACL** — declarative permissions in `goleo.json` (origin/window → allowed
  methods + scopes), enforced centrally in `Bridge` dispatch; deny-by-default for scoped plugins.
- **3b Server hardening (interim B1)** — ✅ **DONE.** Loopback-only bind (`127.0.0.1`),
  prod-strict Origin allow-list on WS upgrade + CORS (dev permissive), per-launch crypto token
  injected into served `index.html` and validated on WS (`?token=`) + `/api/invoke`
  (`X-Goleo-Token`), enforced in production only. Mobile hardened for free (loads injected
  HTML). `runtime/server.go` + `server_test.go`; `bridge/src/bridge.ts` reads/sends the token.
  Known interim limitation: a local process that scrapes `/` can read the injected token — the
  real fix is the native-bind transport (D4), which removes the socket entirely.
- **3c CSP** — configurable Content-Security-Policy for embedded assets.

### D4 — In-process binding, native-bind transport, multi-window & OS integration (XL)
The load-bearing phase; delivers the §1 target. Build against a `WebviewHost`/`Window`
interface (design Windows-first on the proven `edge` layer, then macOS/Linux via purego).
- **4.0 foundation:** `WebviewHost` interface; Windows `edge` impl (multi-window + `goleo://` +
  `Bind`); then macOS (purego, proven) and Linux (purego).
- **Then:** in-process multi-window & custom titlebar → **tray** (`gogpu/systray`, cgo-free) →
  hidden-master lifecycle (`Background`, `ExitOnClose`, `Quit` funnel, daemon) → deep-linking +
  **single-instance** → global shortcuts, autostart, window-state persistence.
- ✅ Retired `webview_go` + the cgo webview permission files → no cgo webview path. (The cgo V4L2
  camera impl, with its pure-Go stub, is the only `cgo`-tagged code left; `CGO_ENABLED=0` uses the stub.)
- Multi-process path demotes to documented fallback.

---

## 3. Track F — Device features (Capacitor-style; absorbed from PLANS.md)

Web UI in a system WebView + a Go provider bridge = Goleo's shape (the Capacitor/Cordova
class). Fill device-feature gaps by extending the host-feature system, porting from Capacitor
plugins as *references*. **Existing (14):** clipboard, dialogs, fs, geolocation, battery, microphone,
wakelock, vibration, sensors, camera, bluetooth, nfc, background, push, + core.

### The vertical-slice pattern (one feature = every touch point)
Reference feature = **`battery`** (has desktop-native + mobile-provider paths). For feature `Foo`:
1. `runtime/foo/foo.go` — `FooInfo`, `Provider`, `SetProvider`, dispatch; tag `//go:build !(android||ios) || goleo_foo`.
2. `runtime/foo/foo_{windows,linux,darwin}.go` — desktop native; unsupported → `errors.ErrUnsupported`.
3. `runtime/foo/foo_mobile.go` (`(android||ios)&&goleo_foo`) + `foo_stub.go` (disabled).
4. `runtime/foo_reexport.go` — `RegisterFoo(b)`, `FooProvider` alias, `SetFooProvider`.
5. `runtime/desktop.go` — add `RegisterFoo` only if on-by-default on desktop.
6. `bridge/src/foo.ts` (+ `index.ts` export) — `invoke` in try/catch with browser fallback.
7. `cli/cmd/scan.go` — `featureRegistry` entry + `scanPatterns` + ref regexes.
8. `cli/cmd/templates.go` — `tmplMobileFooGo` (flat gomobile provider) + generated-file map.
9. `cli/cmd/generate.go` — typed `invoke()` overloads for `goleo:fooXxx`.
10. `cli/cmd/templates/{android,android-dev}/.../MainActivity.java` **and** `ios/.../AppDelegate.swift` — provider wiring (mirror `GoleoBattery`).
11. `create-goleo-app/template/...` — commented `RegisterFoo` + a `FooDemo.vue`.

### Prioritized features
| Feature | Tag | Desktop native | New Android perm? | Capacitor ref |
|---|---|---|---|---|
| **Share sheet** (do first — exemplar) | `goleo_share` | Win share / `NSSharingService` / `xdg-open` | no | `@capacitor/share` |
| **Secure storage** | `goleo_securestore` | wincred / Keychain / libsecret | no | `capacitor-secure-storage` |
| **In-app browser** | `goleo_inappbrowser` | reuse `openURL` | no | `@capacitor/browser` |
| **Biometric auth** | `goleo_biometric` | Windows Hello / Touch ID | no | `capacitor-native-biometric` |
| **Contacts** (do last) | `goleo_contacts` | none | **yes — `READ_CONTACTS`** | `@capacitor-community/contacts` |

**Optional enabler (with Contacts):** wire `featureRegistry.Permissions`/`IOSUsageDescs` into
manifest + `Info.plist` generation (post-process after `extractMobileTemplate()`), closing the
static-manifest gap so future permission-gated features are a pure `scan.go` edit.

### THREE HARD GOTCHAS (do not forget)
- **Manifest permissions are NOT auto-injected** — `scan.go` `Permissions`/`IOSUsageDescs` are
  declared but unread; a feature needing a *new* perm must be hand-added to both `AndroidManifest.xml`
  copies + iOS `Info.plist`.
- **Template duplication** — templates live in `cli/cmd/templates.go` **and**
  `create-goleo-app/src/create-app.ts`; `cli/npm/goleo/` is a full mirror. Mirror every edit,
  rebuild dists (memory: *Goleo template sync*).
- **gomobile marshaling** — `gobind` bridges only primitives/strings; provider interfaces must
  be flat; structs/maps cross as JSON strings; callback features need an `emit*` + shell listener.

  > **Currency note (2026-08-04):** this gotcha is history. `create-goleo-app/` and its
  > `create-app.ts` were deleted on 2026-07-16 — scaffolding is now single-source
  > (minimal in `cli/cmd/templates.go`, demo embedded under `cli/cmd/templates/demo/`),
  > and `cli/npm/goleo/` is generated at publish time by `copy-source.js` rather than
  > being a hand-mirrored tree. There is nothing left to mirror.

---

## 4. Unified execution order (serial)

1. **Commit the built foundation** (multi-window + cgo-free Windows webview + capability guards + docs).
2. **CI mobile-safety guard** — `go build -tags mobilebuild ./runtime/...` in CI (fail fast on desktop-code leaks).
3. **D3b server hardening** — cheap, closes the exposed-port gap now.
4. **F: Share sheet** — smallest device-feature slice; re-proves the vertical-slice pattern.
5. **D2 KV Store** — smallest storage slice.
6. **D1 distribution** — bundler → signing → updater (biggest shipping unlock).
7. **Rest of F** (secure storage, in-app browser, biometric, contacts) + **D3a capability ACL**.
8. **D4** — in-process binding (Windows→macOS→Linux) → native-bind + `goleo://` → in-process
   multi-window → tray → hidden-master lifecycle → deep-link/single-instance → shortcuts/autostart.

Effort legend: S = days · M = 1–2 wk · L = 2–4 wk · XL = 1 mo+ (single-dev, rough).

---

## 5. Cross-cutting rules

**Every plugin/CLI change:**
- [ ] Mirror templates: `cli/cmd/templates.go` **and** `create-goleo-app/src/create-app.ts`; sync `cli/npm/goleo/`; rebuild dists.
- [ ] Typed overloads in `cli/cmd/generate.go`; `scan.go` registry + build tag.
- [ ] PWA/browser fallback verified; `AGENTS.md` updated on architecture change.

**Mobile-safety invariants (never break the gomobile build):**
- [ ] All desktop-binding/window/tray code behind `//go:build !mobilebuild` (+ GOOS). `darwin` ≠ iOS — rely on `!mobilebuild` (gomobile sets it) to keep purego out of iOS.
- [ ] Never call window/tray/desktop-webview code from the `StartServer` (mobile) path.
- [ ] Keep the loopback server + WS bridge as mobile's (and dev-mode's) transport, even after desktop moves to native-bind.
- [x] CI runs the mobile compile guard — **on GOOS=android *and* GOOS=ios** with
  `-tags mobilebuild` (never the host GOOS: `linux + mobilebuild` is unreal and trips
  cgo-only desktop files like `camera_linux.go`).

---

## Decision Log

- **Fork A (windowing): ✅ A2 — richer, CGO-FREE binding** (go-webview2 `edge` on Windows;
  purego WKWebView/WebKitGTK on macOS/Linux). *Corrected from the earlier "cgo-based"
  assumption — the spikes proved cgo-free is viable on all three OSes.* A3 (per-OS hybrid) is a
  fallback only where a platform binding proves too costly.
- **Fork B (transport): ✅ B2 — in-process native-bind, no prod socket** + `goleo://` for
  assets. Achievable only in the in-process model (a cross-process scheme handler would still
  need IPC to the controller). Socket kept for dev HMR + mobile. B1 hardening is the interim
  while the multi-process/socket phase is live.
- **cgo/webview: ✅ SOLVED cgo-free on all three.** Earlier "native webview requires cgo, must
  build per-OS with cgo" is **superseded**. Windows: go-webview2 (`CGO_ENABLED=0` build ✅).
  Cross-compilation restored (darwin cross-built from Windows in Spike 2).
- **Spike 1 (Linux cgo-free `dlopen`): ✅ PASS (2026-07-09).** purego `Dlopen("libgtk-3.so.0")`
  + `gtk_get_major_version()`=3 under `CGO_ENABLED=0` (default, PIE, and cgo) in a `golang:1.26`
  container. `//go:cgo_import_dynamic` makes the CGO_ENABLED=0 binary dynamically linked, so
  `dlopen` works. Remaining Linux work is engineering (GObject signals, webkit versions, `g_idle_add`).
- **Spike 2 (macOS purego → WKWebView): ✅ PASS on real hardware (2026-07-10).** GitHub Actions
  `macos-14` (Apple Silicon), `CGO_ENABLED=0`: a runtime-registered `WKScriptMessageHandler`
  delegate (Go-func method) fired on `postMessage` (JS→Go), `evaluateJavaScript` posted back
  (Go→JS) → `RESULT: PASS`. `CGRect` struct-by-value + nil `completionHandler` worked first try.
  amd64 job + `WKURLSchemeHandler` asset path still to confirm.
- **Multi-window: ✅ implemented (interim, multi-process); in-process is the target (D4).**
  Child-process windows work cgo-free today; in-process hidden-master supersedes it for macOS
  quality + memory + native-bind transport.
- **Lifecycle: ✅ designed** — hidden master, `Background`/daemon, optional `Config.Tray`
  (`gogpu/systray`, cgo-free, verified to build), `WindowOptions.ExitOnClose`, single `Quit()`
  funnel, tracked + OS-parent-death teardown, `app:shutdown` broadcast.
- **Capability guards: ✅ implemented** — `WindowingSupported`/`TraySupported`, `ErrUnsupported`
  guards, `goleo:capabilities`, TS checks. Desktop APIs degrade gracefully on mobile/PWA.
- **SQLite driver:** _TBD — pure-Go `modernc.org/sqlite` preferred._
- **Updater manifest/signing:** _TBD._

---

## Where this stands — resume point (2026-08-10, goleo 0.10.7)

A stopping point, not a finished project. Everything below is verified where it says verified.

### Shipped and verified on real hardware or a real store

- **Desktop** — all three OSes, `CGO_ENABLED=0`, one webview binding (glaze/purego),
  cross-compilable from one machine. Native IPC, scheme assets, in-process multi-window, tray,
  native menu bar, clean shutdown.
- **Android** — signed AAB **accepted by Google Play** (internal track). That upload found a real
  defect no local check could: implied `<uses-feature>` entries defaulting to *required*, quietly
  filtering the app off devices. See `docs/store-submission.md` §1.
- **iOS** — builds, installs and **runs on a simulator** with the full bridge live: Go backend,
  loopback asset serving, WKWebView, `invoke()` and push events. CI-gated in `mobile-verify`.
  Took six never-before-executed defects to reach; every one is in `SPIKES.md` (2026-08-04/05).
  A first **real-device** run (2026-08-09) passed 10 of 14 host features and found four more
  defects — foreground notifications suppressed, dialogs with no provider on *either* mobile
  platform, a silently-nil share presenter, and a CLI device build that produced a *Mac* app.
  All four are fixed; **none is hardware-verified yet.** `SPIKES.md` (2026-08-09).
- **Windows MSIX** — builds and validates with real `makeappx`. Never submitted.
- **Release pipeline** — `release-smoke` installs the *published* CLI with no checkout, on all
  three desktops. It has now caught a real propagation race as well as the three packaging bugs
  it was built for.

### Next, in the order I would pick it up

1. **iOS tier 2** — signed `.ipa` via `archive` + `-exportArchive`, `ExportOptions.plist`,
   entitlements, TestFlight. Blocked *only* on a paid Apple Developer membership; a `macos-14`
   runner does the building. `docs/store-submission.md` §2 names the four files that do not exist
   yet. `goleo build ios --release` currently refuses, with that reason.
2. **Microsoft Store submission** — the artifact exists; needs Partner Center identity and a
   `runFullTrust` justification. The one part that could actually be *refused*, and it has never
   been tested against a reviewer.
3. **Mac App Store acceptance spike** — get a sandboxed hello-world with the loopback bridge
   *accepted* before building any `--mac-appstore` tooling. The App Sandbox forbids the
   self-replacing updater outright, so this may not be viable at all for goleo's architecture.
   **Do not build the tooling first.**
4. Open items above this section: the SQLite driver and the updater-manifest TBDs.

### Loose ends, all deliberate

- The published **v0.10.6 tag has an empty message**. Cosmetic — the release published fine and
  the substance is in the commit history and `RELEASING.md`. Re-annotating means force-pushing a
  published ref, so it was left alone.
- **`mobile-verify` triggers on `runtime/**`**, so a test-only change spends a macOS runner
  (~51 billable minutes at the 10× rate). Narrowing the filter would save that at some cost in
  coverage; not done.
- The **pre-fix device count** for Play versionCode 100 was never captured, so the features fix
  has no before/after. Weighed and declined — see `docs/store-submission.md`.

### If you are picking this up cold

`AGENTS.md` (always loaded) carries the invariants and points at the detail:
`docs/agents/{webview,host-features,desktop-subsystems}.md` behind "read before touching X"
triggers, `docs/store-submission.md` for anything involving a developer account, `SPIKES.md` for
the evidence and failure modes, `docs/history.md` for dated background. Resident context is
~6.4k tokens, down from ~36k, precisely so there is room to work.

---

## Track T — Tauri 2 parity (planned 2026-08-13, goleo 0.11.1)

Gap analysis done against the **source**, not against `docs/comparison.md` (which covers
architecture and philosophy but carries no feature table). Capability claims below come from
`runtime/`, `cli/cmd/schema.go` and `runtime/jsruntime.go`.

### Where goleo stands

| Capability | Tauri 2 | goleo | Note |
|---|---|---|---|
| Multi-window | yes | yes | child **processes**, not in-process windows — native webviews own the GUI thread |
| Tray, app menu | yes | yes | `SetMenu` + tray; **no context menus** |
| Updater (signed) | yes | yes | ed25519 manifest, verified end to end |
| Deep link, single-instance, autostart, store | yes | yes | store is **plaintext JSON** |
| Capability ACL | yes | **partial** | `Policy` enforces methods + `FSRoots`; `HTTPHosts`/`ShellPrograms` are reserved and gate nothing |
| Global shortcuts | yes | **none** | no hits anywhere in the tree |
| Window chrome | yes | **none** | title/width/height only — no decorations, always-on-top, fullscreen, resizable |
| HTTP / shell plugins | yes | **none** | Policy fields exist; no plugin behind them |
| Sidecar binaries | yes | **none** | |
| Isolation pattern | yes | **none** | no sandboxed IPC hop |
| Biometric, barcode scanner | yes | **none** | mobile only |
| cgo-free cross-compile | **no** | yes | goleo's side of the ledger |
| PWA as a build target | **no** | yes | |

### Tier 1 — self-contained, no new security model

- **T1 — Global shortcuts.** System-wide hotkeys that fire when unfocused; the natural companion to
  the existing tray support, since a tray-resident app is otherwise only reachable by aiming at a
  small icon. Three unrelated APIs: `RegisterHotKey` (Win32, purego), `RegisterEventHotKey` (Carbon),
  and on Linux `XGrabKey` for X11 — **Wayland has no global grab by design**, so it needs the
  `org.freedesktop.portal.GlobalShortcuts` D-Bus portal (user-approved, unevenly supported). Expect
  "X11 yes, Wayland where the portal exists, else `ErrUnsupported`".
  Two API requirements: the OS grants a combination **first-come**, so registration can fail and must
  report that rather than silently no-op; and there is **no web equivalent**, so unlike most goleo
  features there is no browser fallback. New `runtime/shortcut/`. Needs a GUI session to verify.
- **T2 — Window chrome options.** The most immediately noticeable absence. Add to both `Config` and
  `createWindow` opts so the JS path stays level with the Go path. The real work is auditing what the
  pinned glaze fork supports — a dependency question before a code one.
- **T3 — Window state persistence.** Cheapest item here; `runtime/store` is the obvious backing. Care
  needed on multi-monitor: a saved position on a monitor that is now gone must clamp on-screen.
- **T4 — Context menus.** Shares the descriptor type with `SetMenu`; the per-platform popup call is
  the new part.

### Tier 2 — valuable, each a security surface

Do **not** start these before the scope model is written. That two of them already have a reserved
`Policy` field suggests the design anticipated them; a shell plugin whose scope enforcement is merely
*intended* is worse than no shell plugin.

- **T5 — HTTP client with host scoping.** Activates `Policy.HTTPHosts`. Enforcement must live in
  `Bridge.HandleRequest` alongside the existing checks, not in the plugin, so a second caller cannot
  bypass it.
- **T6 — Shell / command execution with scoping.** Highest value and highest risk. Precedent to
  follow: `goleo:openURL` is scheme-allow-listed because an OS handler would otherwise open
  executables. This needs the same rigour an order of magnitude harder — argv arrays never command
  strings, an allow-list of programs rather than a deny-list of characters, no shell interpolation
  anywhere. Design review before code.
- **T7 — Sidecar binaries.** Matters enormously if needed and not at all otherwise; confirm demand
  first. Touches the bundler for every target plus path resolution inside an installed app. Should
  depend on T6's execution model rather than duplicating it.

### Tier 3 — on demand

- **T8 — Encrypted store** (stronghold equivalent). `runtime/store` is plaintext, fine until someone
  puts a token in it.
- **T9 — Utility plugins**: CLI arg parsing, log, positioner, upload, websocket client. Each small;
  none is why anyone picks a framework.
- **T10 — Mobile: biometric, barcode scanner.** Both fit the existing `Provider` pattern; both need a
  device. **The barcode half is superseded by P2** (Track P) — plan it there, not here; T10 is now
  biometric only.
- **T11 — Security depth: scopes and the isolation pattern.** The honest structural gap. Research
  before implementation, and it should **follow** T5/T6 — those will show what the scope model needs
  to express.

**Deliberately not planned:** multi-webview per window (unstable upstream, and it conflicts with the
child-process window model). Haptics and geolocation are already covered by `vibration` and the web
API.

---

## Track J — Make the JS backend usable (planned 2026-08-13, DONE 2026-08-13)

> **Status: complete.** J1, J2, J3, J4, J5 and J6 all shipped. What landed differs from
> what was planned in one important way — see J2. Kept as written, with outcomes marked,
> because the reasoning is the point of the entry.
>
> | Item | Outcome |
> |---|---|
> | J1 | done — both scaffolds document the real API; the fabricated `bridge.invoke` block is gone |
> | J2 | **decided: both.** Bootstrapper *and* scripting layer, primary direction Go → JS |
> | J3 | done, both directions — `app.JS().Call` and `goleo.invoke`/`goleo.emit` |
> | J4 | done — the two directions were split; Go → JS needs no ACL, JS → Go routes through `Bridge.HandleRequestContext` |
> | J5 | done — `goleo generate types` emits `backend/init.d.ts`; verified with `tsc` |
> | J6 | done — three guards hold the VM, both comment blocks and the `.d.ts` to one global set |
>
> The finding worth carrying forward: **goja is not goroutine-safe and `jsruntime.go` had no
> locking at all**, which was safe only because `Run()` was called once at startup. The VM now
> has one owning goroutine. Its single hazard — JS → Go → JS deadlocking on that goroutine —
> is closed by a context marker that makes a nested call run inline; removing it makes the
> test hang rather than fail, so it is guarded by a timeout.


### The defect that prompted this

Both scaffolds' `backend/init.js` ship ~35 lines documenting `bridge.invoke("goleo:...")`. The VM
(`runtime/jsruntime.go`) defines exactly three globals:

```
jsr.vm.Set("createWindow", ...)
jsr.vm.Set("getConfig",    ...)
jsr.vm.Set("console",      ...)
```

There is **no `bridge` object**. Every documented call fails with `ReferenceError: bridge is not
defined`. The block also still lists `goleo:geolocationGetCurrentPosition`, removed in 0.11.0.

This is the repo's recurring shape inverted: usually a declaration with no consumer, here
documentation for a declaration never made.

So "a JavaScript file can take over and expose features" currently means **it can create windows**.
That is genuinely useful — multi-window layout without touching Go — but it is the whole of it.
Missing versus the Go API: `CloseWindow`, `ListWindows`, `SetMenu`, tray, `Emit`/`On`, invoking any
of the 60 bridge commands, registering a handler, and `Quit`.

### Tasks, in dependency order

1. **J1 — Stop shipping instructions that cannot work.** Rewrite the comment block in both scaffolds
   to describe the three real globals; delete the fabricated `bridge.invoke` section. Independent of
   every decision below and worth doing alone — a developer's first hour currently goes to debugging a
   `ReferenceError` against documentation we wrote. ~30 minutes.
2. **J2 — DECIDED (2026-08-13): both.** `init.js` stays a window bootstrapper *and* gains a
   scripting layer, with the primary direction **Go → JS**: Go code calls JS functions the script
   defined. That is the opposite of what J3 originally assumed (JS reaching into the bridge), and it
   changes the design — see J3 below, which is rewritten for it. The bootstrapper role is unaffected;
   `createWindow`/`getConfig` keep working exactly as they do now.
3. **J3 — Go → JS invocation (rewritten for the J2 decision).**

   **The constraint that dictates the design: `goja.Runtime` is NOT goroutine-safe, and
   `jsruntime.go` has no locking at all.** That is safe today only because `Run()` is called once,
   from one goroutine, during startup. Bridge handlers run one goroutine per request, so the first
   `app.JS().Call(...)` from a handler is a data race — not a subtle one.

   So the VM gets a **single owning goroutine** and callers submit jobs to it over a channel,
   replying on a per-call channel. A plain mutex is the tempting alternative and is wrong: JS
   calling back into Go which calls JS again deadlocks a non-reentrant mutex, and that re-entry is
   exactly what a scripting layer invites.

   Shape:

   - `app.JS().Call(ctx, "fnName", args...) (any, error)` — call a global function defined by the
     init script. Returns a Go value, or an error carrying the JS exception.
   - **Marshal via JSON**, not goja's native struct mapping. Same lesson as the gomobile providers:
     a predictable JSON boundary beats a reflective one that silently omits what it cannot map.
   - **Every call takes a context.** A runaway script (`while(true){}`) would otherwise wedge the
     owning goroutine forever; `vm.Interrupt()` is the escape and needs a deadline to fire on.
   - **A JS exception is a Go error**, never a panic crossing the boundary.
   - `Stop()` must drain and refuse queued calls rather than leaving callers blocked forever.

   Deliberately NOT in this step: `emit`/`on`, `setMenu`, `quit`, `closeWindow`, `listWindows`, and
   JS→Go `bridge.invoke`. They are the reverse direction, each is a new bridge surface, and J4
   applies to them rather than to Go → JS.
4. **J4 — `Policy` and the two directions.** These are not the same question, and conflating them
   is how an ACL gets holed.

   **Go → JS (J3) is not a bridge surface and needs no ACL.** Go already has unrestricted access to
   everything `Policy` protects; calling a JS function it chose to call adds no capability. Gating it
   would be theatre.

   **JS → Go (a later step) absolutely does.** When `bridge.invoke` is eventually bound, route it
   through `Bridge.HandleRequest` (`runtime/bridge.go`, ~line 121) rather than the handler map, so
   the capability check applies for free and cannot drift. A second path into that map is precisely
   how an ACL gets bypassed later unnoticed.

   The asymmetry is worth stating in the code: init.js is app-author code, same trust level as
   `backend/app/app.go`, so it is not the thing `Policy` defends against — the frontend is.
5. **J5 — Generate `init.d.ts`.** `goleo generate types` already emits typed `invoke()` overloads for
   the frontend from `KnownCommands`; the same generator and source can emit a backend declaration
   file. Note what it would have prevented: with a generated `.d.ts`, the phantom `bridge` object would
   have shown as undefined the moment anyone opened the file.
6. **J6 — Assert the docs against the VM.** A test that every global named in the scaffold's comment
   block exists in the runtime, and the reverse. This is the guard whose absence let a fabricated API
   ship — the same declaration-with-no-consumer shape as the microphone defect, the unread iOS usage
   descriptions, and the stale `goleo.d.ts`.

### Suggested order across both tracks

- **Now, independent of everything:** J1.
- **Next, cheap and visible:** T3 and T1 — the two users notice, neither needing a design debate.
- **Then the decision:** J2. Unblocking, and it costs a conversation rather than a sprint. If the
  answer is "scripting layer", J3–J6 follow and J5 pays back immediately.
- **Only then:** T5/T6 with a written scope model, and T11 after them.

> Tauri details reflect 2.x as known on 2026-08-13; its plugin workspace moves quickly, so spot-check
> anything before acting on it.

---

## Track P — native push and barcode scanning (planned 2026-08-13, from an external proposal)

Origin: a `TASK.md` gap analysis produced by another agent. Two of its three items are real gaps
and are planned below. The rest was assessed and **rejected**; the verdicts are recorded here so
the same proposal does not get re-litigated.

### Assessment of the source proposal

| Claim | Reality | Verdict |
|---|---|---|
| Remote push (APNs/FCM) missing | `runtime/push` is **Web Push** — `Subscribe(serverKey)` → `PushSubscription`, i.e. VAPID. Device-token push genuinely absent | **valid — P1** |
| Barcode/QR scanning missing | `camera.Provider` is `CapturePhoto`/`StartStream`/`StopStream` only | **valid — P2** |
| Transparent webview for a native scan overlay | No transparency support today | **valid — prerequisite of P2, = T2** |
| `goleo init`, `goleo mobile init` | `goleo new` exists; native shells are generated into `.goleo/{android,ios}` on every build | already built |
| `goleo build desktop --os=`, `goleo build mobile --target=` | `goleo build windows\|linux\|darwin\|android\|ios` | already built |
| "inject asset bundles into the native asset directory" | Deliberately not done: the frontend is embedded in the Go library and served over loopback, because `file:///android_asset` is not a secure context and a native copy duplicated the whole frontend into every artifact | contradicts a solved problem |

**Rejected outright, with reasons:**

- **CGO bindings** ("Go-Mobile/CGO bindings", "CGO/Objective-C wrapping `UNUserNotificationCenter`").
  Violates the core invariant — `CGO_ENABLED=0` everywhere, every desktop target cross-compilable
  from one machine — and is unnecessary. Mobile providers live in the native Swift/Java shell and
  are registered via gomobile reverse bindings; nine already work that way. APNs registration
  belongs beside `GoleoNotifier` in `AppDelegate.swift`.
- **A `permissions` block in `goleo.json`.** Android permissions are DERIVED from the `Register*`
  calls the scanner finds. A hand-written list is exactly what that design replaced, and it
  reintroduces the defect where every app declared thirteen permissions it did not use.
- **`window.__goleo.*` as a new global.** Bypasses `@goleo/bridge` and, critically,
  `Bridge.HandleRequest` — the one place the `Policy` ACL is enforced — and collides with the
  existing `window.__goleoRecv` / `__goleoOnMessage` / `__goleoDrain` native-IPC internals.
- **The proposed `goleo.json` schema** (`name`, `appID`, `frontend.dist`, `devURL`) is incompatible
  with the shipped one (`app_name`, `bundle.identifier`, `frontend.directory`/`dist_dir`,
  `mobile.android.package_name`). Adopting it breaks every existing project.
- **Replacing the push `Provider` interface.** The proposal swaps `Subscribe`/`Unsubscribe`/
  `GetSubscription` for `RegisterRemote`/`GetToken`/… That is a breaking change to a shipped API
  with live bridge commands and a TS wrapper. Native push is ADDITIVE (P1), not a replacement.

### P1 — native remote push (device-token), alongside Web Push

Web Push already works and stays. This adds the device-token path that mobile stores actually use.

- **Additive interface.** A separate `RemoteProvider` in `runtime/push/`, not a rewrite of
  `Provider`. Existing `Subscribe`/`Unsubscribe`/`GetSubscription` and the three `goleo:push*`
  bridge commands are unchanged.
- **New commands** follow the existing naming: `goleo:pushRegisterRemote`,
  `goleo:pushGetDeviceToken`. Token-refresh and notification-received are **events**
  (`app.Emit`), matching how the rest of the runtime pushes to the frontend — not callbacks
  injected into a new JS global.
- **iOS**: `registerForRemoteNotifications()` + `didRegisterForRemoteNotificationsWithDeviceToken`
  in `AppDelegate.swift`, registered like the other nine providers. **No cgo.** Needs the
  `aps-environment` entitlement, which the generated project does not currently emit — that is
  the real work, and it needs a **paid** Apple membership to test, so it is blocked on the same
  account as everything else in `docs/store-submission.md`.
- **Android**: FCM via the Gradle template + a `GoleoPushProvider` in the shell. Requires
  `google-services.json` from the app developer, so it needs a config path and a clear error when
  absent.
- **Permissions**: `POST_NOTIFICATIONS` is already core. FCM adds nothing on modern Android, but
  check the derived manifest against a real build (`aapt2 dump permissions`) rather than the
  template — the manifest merger adds entries goleo never declares.
- **Desktop**: out of scope. The proposal lists "FCM for Desktop"; there is no such thing outside
  Chrome. Desktop keeps Web Push.

### P2 — barcode / QR scanning

Supersedes the barcode half of **T10**. Camera scanning is the higher-value half; biometric stays
in T10.

- **Additive interface** again: `ScanProvider` alongside `camera.Provider`, so `CapturePhoto` and
  the stream methods keep working.
- **Desktop and PWA**: `BarcodeDetector` where available, and state plainly that it is
  **Chromium-only** — Safari and Firefox do not ship it, so the honest fallback is
  `ErrUnsupported` and a JS-side decoder chosen by the app, not a promise goleo cannot keep.
- **Mobile**: native scanners (ML Kit on Android, `AVCaptureMetadataOutput` on iOS) in the shells.
  Both are genuinely better than a canvas loop, which is the proposal's one solid technical point.
- **Blocked on T2 (window chrome).** A native preview layer behind a transparent WebView needs
  transparency support, which does not exist. Do T2 first, or ship a full-screen native scan view
  with no web overlay as a simpler first cut.
- **Verify on a device.** Neither scanner can be checked from a non-Mac, non-device host — the same
  constraint as every other mobile feature.

### Sequencing

P2 before P1: barcode scanning is self-contained, has a real desktop story, and needs no developer
accounts. P1's iOS half is blocked on a paid Apple membership and its Android half needs a Firebase
project, so it should not be started until someone can actually test it end to end.

Neither belongs in Tier 1 of Track T — both are larger than the four items there, and both need a
device to verify.
