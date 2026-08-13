# Desktop subsystems: windowing, lifecycle, transport, distribution

> Split out of `AGENTS.md`, which loads into every session. **Read it before changing windowing/lifecycle (runtime/window*.go, tray, menus), the native-IPC or scheme-asset transports, OS integration, or the bundling/publishing path in cli/cmd/.**
>
> `AGENTS.md` carries the invariants that must be known even without reading this;
> everything here is the detail behind them. `SPIKES.md` has the evidence and the
> hardware-verification history.

---
## Desktop subsystems (windowing, lifecycle, distribution, security)

Added on top of the core bridge/feature system. Full rationale + status in
`docs/roadmap.md` (the masterplan); feasibility findings in `SPIKES.md`.

### Windowing
- **Multi-process (default, cross-platform):** `runtime/windowmanager.go` `WindowManager` spawns
  each extra window as a child process (`runtime/window_child.go`, `GOLEO_WINDOW=1`) that hosts
  one webview against the shared server. The primary window is hosted in-process by `runWebview`.
- **In-process (Windows, opt-in):** `inProcWindowManager` hosts each window on its own
  `LockOSThread` goroutine (proven in `spikes/win-multiwindow/`). Selected by
  `Config.InProcessWindows` on Windows. **macOS/Linux** are main-thread-only, so extra
  in-process windows share the primary's single run loop (`mainLoopWindowManager`,
  `runtime/app.go`) rather than getting their own. All implement `windowSpawner`.
- API: `App.OpenWindow/CloseWindow/ListWindows`, bridge `goleo:window{Open,Close,List}`,
  `bridge/src/window.ts`; `WindowOptions.ExitOnClose` quits the app when that window closes.
- **Geometry, chrome and state:** `Config.Chrome` / `WindowOptions.Chrome` /
  `WebviewWindow.{Rect,SetRect,SetChrome}` / `App.RememberWindowState` (`runtime/windowgeom*.go`,
  `runtime/windowstate.go`). glaze has no API for any of it — it is purego against the native
  handle, per platform. **Read `docs/agents/webview.md` before changing it**: the `*bool` fields,
  the four surfaces they must stay in sync across, and why the state restore happens in
  `runWebview` rather than where it is called are all recorded there.

### Lifecycle
- `App.Quit()` — single idempotent shutdown funnel (unblocks the run loop → `CloseAll` →
  `OnShutdown` → stop server); `Stop()` is an alias; `goleo:quit` / `quitApp()`.
- `Config.Background` — headless controller: no auto primary window; main thread runs the tray
  (if set) or blocks until Quit. `Config.OnReady` runs after the server + window manager are up
  (where `OpenWindow` works, unlike `OnStartup`).
- **Tray:** `Config.Tray` (`TrayConfig`/`TrayItem`), cgo-free on all desktops. Windows/Linux use
  `github.com/gogpu/systray` (`runtime/tray_desktop.go`, `!darwin && !mobilebuild && !js`); **macOS**
  uses a `purego`/objc `NSStatusItem` backend (`runtime/tray_darwin.go`) — necessary because
  systray's `goffi` and glaze's `purego` each export `_cgo_init` and collide at Mach-O link time, so
  macOS reuses glaze's FFI instead of importing systray. `tray_stub.go` on mobile/wasm. See `SPIKES.md`.
- **Native menu bar (all three desktops):** `Config.Menu` / `App.SetMenu([]MenuItem)`
  (`runtime/menu.go`). `MenuItem` has `Label`, `Role`
  (`RoleQuit/Copy/Paste/SelectAll/Undo/Redo/Cut/Minimize/Close`), `Accelerator` (`"cmd+q"`…),
  `OnClick`, `Submenu`, `Separator`. Backends, all cgo-free via `purego`:
  - **macOS** (`menu_darwin.go`): `NSMenu` set as `NSApplication.mainMenu` (objc); roles go up the
    responder chain so Cmd+C/V/X/A/Z work in the webview; auto-installs `StandardMenu(Title)` when
    `Config.Menu` is empty.
  - **Windows** (`menu_windows.go`): user32 `HMENU` `SetMenu` on the HWND + a wndproc subclass for
    `WM_COMMAND` clicks; roles use `execCommand` (WebView2 handles the Ctrl shortcuts itself).
  - **Linux** (`menu_linux.go`): reparents the webview under a `GtkBox`. **GTK3** (webkit2gtk-4.x):
    `GtkMenuBar` + `GtkMenuItem` + accelerators (`GtkAccelGroup`). **GTK4** (webkitgtk-6.0, no
    `GtkMenuBar`): GMenu model + `GtkPopoverMenuBar` + `GActions` inserted on the window. Picks the
    stack glaze loaded (RTLD_NOLOAD). Accelerators: functional on GTK3; GTK4 is best-effort.
  - PWA/mobile: `SetMenu` returns `errors.ErrUnsupported`; `MenuSupported()` /
    `goleo:capabilities.menu` report false (`menu_other.go`).
  - **Bridge API:** `goleo:setMenu` (`app.go`) + `@goleo/bridge` `setMenu()`/`onMenu(id,cb)`
    (`bridge/src/menu.ts`) — a frontend menu tree; leaf items with an `id` emit `menu:<id>` events.
  - Verified: Windows (local GUI), Linux (Docker/xvfb), macOS (`macos-14`) via `spikes/glaze-menu-verify`.

### OS integration
- **Single-instance** (`runtime/singleinstance/`): first launch binds a per-app loopback address;
  later launches forward args (ACK-handshaked) and exit, emitting `app:secondInstance`. Opt-in
  via `Config.SingleInstance` (+ `AppID`). Pure Go, cross-platform.
- **Autostart** (`runtime/autostart/`): Windows HKCU Run key (`x/sys/windows/registry`), macOS
  LaunchAgent plist, Linux `~/.config/autostart` .desktop. `goleo:autostart{Enable,Disable,IsEnabled}`.
- **Deep links** (`runtime/deeplink/`): register a `myapp://` scheme (Windows registry, Linux
  `x-scheme-handler` .desktop, macOS via the bundler's `CFBundleURLTypes`). `Config.URLScheme`;
  launch URL via `goleo:initialURL`, later launches → `app:openURL` (through single-instance).

### Transport
- **Native in-process IPC** (`runtime/nativeipc.go`, opt-in via `Config.NativeIPC`): a natively
  hosted window talks to the `Bridge` over the webview's own channel instead of the loopback
  WebSocket. Each such window owns a `nativeSession`. `nativeOnInit` (wired through
  `windowConfig.OnInit`, pre-navigation) injects a shim (`window.__GOLEO_NATIVE__` / `__goleoRecv`)
  and binds `__goleoSend` (Go func); the session is stashed on `WebviewWindow.sess`.
  `session.onMessage` decodes the same `{type,data}` envelope as `websocket.go` and funnels into
  `Bridge.HandleRequest` (so `Policy` still applies); invokes run on their own goroutine to keep
  off the UI thread. Backend→frontend frames are pushed via `Eval(window.__goleoRecv(...))` on the
  UI thread (`session.startEventPump` replaces the WS hub per window). `Bind`/`Init`/`evaler()`
  added to all `WebviewWindow` backends (`webview_glaze.go`, `webview.go`, `webview_stub.go`).
  - **Coverage:** the primary window (`runWebview`, incl. the `init.js` `createWindow` window) **and
    in-process additional windows** (`Config.InProcessWindows`, `windowmanager.go`) — each gets its
    own independent session. Child-*process* windows, browser/PWA and mobile keep using WebSocket
    (`@goleo/bridge` auto-detects the native channel, else falls back). The HTTP/WS server stays up:
    it still serves embedded assets and is the fallback transport. Dropping it too via custom-scheme
    (`goleo://`) asset serving is **implemented on all three desktops via `Config.SchemeAssets`**
    (see below). See "Scheme assets" under Desktop subsystems.
  - **Verified** on real WebView2 (Windows, cgo-free): a two-window app where each window completes
    an independent bidirectional round-trip over its own native channel, incl. `goleo:windowOpen`
    over native IPC, then a clean `Quit`. Also `runtime/nativeipc_test.go` (round-trip, policy,
    events, ping, pump-stop) + `bridge` tsc.
- **Scheme assets** (`Config.SchemeAssets`, opt-in; `runtime/scheme_assets.go`): serves the primary
  window's embedded UI from a portless, secure custom origin (`Config.AssetScheme`, default
  `goleo://`) instead of the loopback HTTP server. With `NativeIPC` on, that window opens **no TCP
  port at all** while keeping a secure context (localStorage / crypto.subtle / getUserMedia /
  history routing). Takes effect only in production (embedded FS, not `DevMode`) via the glaze
  `SchemeHandlers` API (`newGlazeWebView` in `webview_glaze.go`) — now on **all three desktops**
  since the Windows→glaze migration: macOS/Linux serve the literal `goleo://` scheme, **Windows**
  serves it over a secure `https://<scheme>.localhost` virtual host (WebView2 has no per-scheme
  secure flag; `Navigate` rewrites `goleo://` to the vhost so callers are platform-agnostic). A
  shared `buildAssetServer` resolves request paths to bytes+MIME from
  `frontend/dist` with SPA index fallback and bridge-token injection. The loopback server stays up
  as the fallback transport. Verified end-to-end on Linux + `macos-14` (`goleo://app`) and Windows
  (`https://goleo.localhost`) via `spikes/goleo-scheme-verify`. The `NewWithOptions`/`SchemeHandlers`
  API this needs is **upstream** as of `crgimenes/glaze` `v0.0.46`, so scheme assets no longer
  require the fork (the fork is still pinned, but only for the Windows permission auto-grant — see
  "Why goleo pins a glaze fork" above). Because Go `replace` directives don't transit, **any
  downstream module importing goleo's runtime still needs the same replace** to inherit that grant —
  `goleo new` scaffolds it into the generated `go.mod`. See `SPIKES.md` (2026-07-13, 2026-08-03).

### GUI lifecycle threading (fixed alongside native IPC)
Two pre-existing defects surfaced by driving `Quit()` end-to-end:
- **`a.ctx` was clobbered:** `Run` installed a cancellable context, then `StartServer` overwrote
  `a.ctx` with a fresh `context.Background()`, orphaning `a.cancel()`. `Quit` cancelled a context
  nothing watched, so shutdown hung. `StartServer` now keeps an existing `a.ctx` (only defaults to
  `Background` when nil, i.e. the standalone/mobile entry).
- **Main goroutine not thread-pinned:** the native webview is thread-affine (its window messages
  and `Dispatch` target the creating thread), but the Go main goroutine can migrate OS threads
  between window creation and `Run`, so cross-thread teardown missed. `Run` now calls
  `runtime.LockOSThread()` up front so the whole GUI lifecycle stays on one thread (matching what
  the in-process `WindowManager` goroutines already did).

### Distribution (CLI, `cli/cmd/`)
- `bundle.go` — `goleo build --bundle`: NSIS (Windows, auto-installs `makensis` via winget/choco/
  scoop), `.app`+`.dmg` (macOS), `.deb`/`.rpm` (nfpm, Linux — with a generated hicolor icon +
  `.desktop` entry). The Windows installer registers with Add/Remove Programs so the app is
  uninstallable from Settings. `signing.go` — env-driven Authenticode / codesign+notarytool.
- `msix.go` — `--windows-format msix|both` packages for the **Microsoft Store**: an
  `AppxManifest.xml` declaring `Windows.FullTrustApplication` + the restricted `runFullTrust`
  capability (which is what keeps the loopback bridge working — a full-trust package is not
  sandboxed), logo assets generated from the one `bundle.icon`, `makeappx pack` located under
  `Windows Kits`, then the existing signtool path. Identity comes from `windows.msix` in
  goleo.json and is validated rather than guessed: a wrong Name or Publisher builds and signs
  happily, then fails at install or submission with nothing pointing at the cause. Verified
  end to end on Windows — a real `.msix` whose manifest parses, with correct XML escaping and
  full-trust declarations.
- **Icons (`icons.go`, pure Go — no ImageMagick/iconutil):** one `bundle.icon` PNG (≈1024²) is
  area-averaged/re-encoded into every artifact — multi-size Windows `.ico` (embedded via
  `winres.go`/goversioninfo), macOS `.icns`, Linux hicolor PNG, Android `mipmap-*/ic_launcher(+
  _round).png`, iOS `AppIcon.appiconset`. Explicit `icon_ico/icns/png` override. Mobile icons are
  injected into the extracted project after `extractMobileTemplate` and referenced from the manifest/
  xcodegen only when a source icon resolves (`mobileConfig.HasIcon`). Unit-tested in `icons_test.go`.
- `publish.go` — `goleo build --publish`: stages a platform artifact, SHA256s it, and merges a
  `Release` into an ed25519-signed `manifest.json` (`updater.SignManifest`). `generate updater-key`.
- **Updater** (`runtime/updater/`): `RegisterUpdater(b, UpdaterConfig{ManifestURL, PublicKey,
  CurrentVersion})`; `goleo:updater{Check,Apply}` verify the signed manifest before applying.

### Storage
- **KV store** (`runtime/store/`): `RegisterStore`; JSON file in the app-data dir, atomic writes;
  `goleo:store{Get,Set,Delete,Keys,Clear}` + `bridge/src/store.ts` (localStorage fallback).

### Capability guards
- `runtime/capabilities*.go`: `WindowingSupported()`/`TraySupported()` + `errors.ErrUnsupported`
  guards; `goleo:capabilities` query. Desktop-only APIs degrade gracefully on mobile/PWA.
