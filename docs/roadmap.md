# Goleo Masterplan

> The single authoritative plan for Goleo's evolution. Consolidates the former
> desktop-parity roadmap and the device-feature plan (`PLANS.md`, now absorbed).
> Two tracks run in parallel:
> - **Track D — Desktop parity & architecture** (windowing, lifecycle, distribution, security, transport).
> - **Track F — Device features** (Capacitor-style host features on desktop + mobile).
>
> Cold-start orientation: read [`AGENTS.md`](../AGENTS.md), then this file.

---

## 0. Current status (what is built vs designed)

**Built & verified (uncommitted WIP unless noted):**
- **cgo-free Windows webview** — `runtime/webview_windows.go` uses `github.com/jchv/go-webview2`
  (WebView2 via COM/syscall). `CGO_ENABLED=0 GOOS=windows go build ./runtime/...` ✅.
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
  layer), macOS `purego`+WKWebView, Linux `purego`+WebKitGTK. Reference implementation for the
  ObjC/GTK call surface = **Wails v3** source (its `.m`/`.c` files are the API spec; Wails is
  cgo, we port cgo-free).
- **In-process multi-window** under the master's run loop (Tauri/Wails model). Multi-process is
  the interim/fallback (works today with minimal bindings; the reason it can't be the end state
  is macOS dock/menu fragmentation + memory).
- **Native-bind IPC** (`go-webview2 Bind` / WKScriptMessageHandler / WebKit message handler) —
  **no loopback socket in production**. Socket retained only for **dev-mode HMR** and **mobile**.
  Custom `goleo://` scheme serves embedded assets.
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
- Retire `webview_go` (and the cgo permission file) → whole project builds `CGO_ENABLED=0`.
- Multi-process path demotes to documented fallback.

---

## 3. Track F — Device features (Capacitor-style; absorbed from PLANS.md)

Web UI in a system WebView + a Go provider bridge = Goleo's shape (the Capacitor/Cordova
class). Fill device-feature gaps by extending the host-feature system, porting from Capacitor
plugins as *references*. **Existing (13):** clipboard, dialogs, fs, geolocation, battery,
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
