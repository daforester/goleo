# Goleo Framework — AI Context File

## Overview

Goleo is a Go-based framework for building cross-platform desktop and mobile applications using Go for the backend and web technologies for the frontend. It supports **Windows, Linux, macOS, Android, and iOS** from a single codebase.

**Core concept**: Write your app logic in Go, build your UI with any web framework (Vue, React, Svelte, vanilla JS, etc.), and Goleo handles the bundling, communication bridge, and platform-specific packaging.

> **Doing anything with the Google Play, Apple, or Microsoft developer accounts?**
> Read [`docs/store-submission.md`](docs/store-submission.md) first. **Play has accepted a signed
> AAB** (internal track, 2026-08-05) — and that upload immediately found a defect nothing local
> could catch, which is why it is worth doing rather than reasoning about. The Microsoft Store
> and Apple paths have **never** had an artifact accepted; neither is blocked on code (MSIX needs
> Partner Center identity and a restricted-capability justification, iOS needs a paid Apple
> membership). That doc carries the procedures, what is proven versus unverified, and the
> platform gotchas — package names being permanent, `keytool` not being on PATH, XcodeGen
> silently overriding settings, the Mac App Store sandbox forbidding the updater. It exists
> because that work will be picked up long after this context is gone.

## Repository Structure

`ls`/`find` the tree rather than reading it here. Two things it will not tell you:
`templates/` is EMPTY (the scaffold lives in `cli/cmd` — minimal in `templates.go`, demo
embedded under `cli/cmd/templates/demo`), and `cli/npm/goleo/` is a GENERATED copy of the
root produced at publish time, not a separately-maintained module.

## Architecture

### Communication Flow

The frontend (browser/webview) communicates with the Go backend over one of three
transports, selected automatically by `@goleo/bridge` in this priority order:

- **Native in-process IPC** (opt-in, `Config.NativeIPC`; preferred when present):
  the desktop webview host injects a message channel (a bound Go function for
  frontend→backend, evaluated JS for backend→frontend) so the primary in-process
  window talks to the `Bridge` directly — no socket, no port. See "Native IPC"
  under Desktop subsystems.
- **WebSocket**: Persistent bidirectional connection. The default transport, and
  the mandatory backbone for child-process windows, browser/PWA, and mobile. Low
  latency, supports server push events.
- **HTTP POST** (fallback): Calls /api/invoke when WebSocket is unavailable. No event push support.

All three carry the same `{id, method, args}` / `{id, result|error}` envelopes and
funnel through the same `Bridge.HandleRequest` (so the `Policy` ACL applies
uniformly); the bridge falls back down the list transparently.

### Request/Response Flow

Frontend sends an invoke message with {id, method, args}. The Go Bridge matches the method to a registered handler, calls it, and returns {id, result} or {id, error}.

Events flow from backend to frontend (push) via WebSocket, or from frontend to backend as one-way messages.

### Dev Mode

- Frontend runs on Vite dev server (port 5173) with HMR (hot module replacement)
- Go backend runs on port 9842
- Vite proxies /api/* and /ws to Go backend
- Changes to frontend code trigger instant HMR without page refresh
- Changes to Go backend require restart (planned: live reload via air)

### Production Build

1. Frontend is built with Vite into frontend/dist/
2. The dist/ directory is embedded into Go binary via //go:embed
3. Go binary serves embedded static files along with API on the same port
4. A single self-contained executable is produced

## Go Runtime Library (runtime/)

The runtime package is imported by user applications.

### App Lifecycle

`runtime.New(Config{...})` then `app.Run()` (blocks until SIGINT/SIGTERM). See
`runtime/app.go`; the Config fields worth knowing are below.

### Config Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| Title | string | "Goleo App" | Window title |
| Width | int | 1024 | Window width |
| Height | int | 768 | Window height |
| DevMode | bool | false | Enable dev mode (CORS, no embedded files) |
| DevServer | string | "" | Frontend dev server URL |
| Port | int | 9842 | Server port. 0 means "use the default": `New()` maps it to 9842 (`runtime/app.go`). The server then falls forward to the next free port if 9842 is taken. |
| WindowMode | WindowMode | WindowModeBrowser | Display mode (browser/webview/mobile) |
| EmbedFS | any | nil | Embedded frontend filesystem |
| OnStartup | func(ctx) | nil | Startup callback |
| OnShutdown | func(ctx) | nil | Shutdown callback |

### Bridge API (Go side)

`app.Bridge().Handle(name, func(ctx, json.RawMessage) (any, error))` to register;
`app.Emit(event, payload)` / `app.On(event, handler)` for events. The built-in
`goleo:*` commands are whatever `RegisterBuiltins` and the `Register*` feature
functions install — grep `runtime/` for `Handle(` rather than trusting a list here,
which has drifted before. `goleo generate types` emits the authoritative set as
typed `invoke()` overloads.

### Server Endpoints

Routes are declared in `runtime/server.go` (`/api/invoke`, `/ws`, `/api/health`, and
static file serving at `/` in production).

The server auto-selects a port if the configured one is in use and sets CORS headers for all origins in dev mode.

## CLI Tool (cli/)

### Commands

| Command | Description |
|---------|-------------|
| goleo new <name> | Scaffold a new Goleo project (prompts for minimal vs demo; `--demo` / `--template`) |
| goleo dev | Start development mode (Go + Vite with HMR) |
| goleo dev pwa | Start PWA development mode (Vite only, no Go backend) |
| goleo build | Build for current platform |
| goleo build windows | Cross-compile for Windows amd64 |
| goleo build linux | Cross-compile for Linux amd64 |
| goleo build darwin | Cross-compile for macOS amd64 |
| goleo build android | Build an unsigned debug .apk (gomobile AAR + Gradle) |
| goleo build android --release | Build a **signed .aab** for Play (`--android-format apk` for a signed APK) |
| goleo build ios | Build a debug iOS `GoleoApp.app` for a **device** (gomobile xcframework + xcodegen + xcodebuild). Needs `mobile.ios.development_team` (or `--ios-team`); refused early without one |
| goleo build ios --simulator | Build an **unsigned Simulator** app — the only iOS path that needs no Apple Developer account |
| goleo build pwa | Build Progressive Web App (no Go backend) |
| goleo build --bundle | Also package the desktop app into a native installer (dist/bundle/) |
| goleo build --publish | Also write an ed25519-signed update manifest (needs GOLEO_UPDATE_PRIVKEY) |
| goleo emulate android | Run in dev mode on a connected Android device or emulator |
| goleo install android | Sideload the built app.apk onto a connected device + launch it |
| goleo generate types | Generate frontend/src/goleo.d.ts (typed invoke() overloads) |
| goleo generate updater-key | Generate an ed25519 keypair for signing update manifests |
| goleo generate android-key | Generate an Android signing keystore (uses the JDK goleo resolves, so keytool need not be on PATH) |
| goleo doctor android | Report whether the android build/emulate dependencies are present — discovery only, installs nothing and never prompts (so CI and editor tooling can check readiness safely) |
| goleo version | Print version |

### Build Targets

| Target | GOOS | GOARCH | Output | Dependency |
|--------|------|--------|--------|------------|
| current | auto | auto | `app.exe` on Windows, `app` elsewhere | none |
| windows | windows | amd64 | .exe | none |
| linux | linux | amd64 | binary | none |
| darwin | darwin | amd64 | binary | none |
| android | android | arm64 | `app.apk` (debug) or `app.aab` (`--release`) | gomobile + NDK |
| ios | ios | arm64 + arm64-simulator | `GoleoApp.app` (debug; the `.xcframework` is an intermediate and is deleted) | gomobile + Xcode (+ a Team ID for device builds) |
| pwa | js | wasm | dist-pwa/ | none |

### Build-flag validation (`validateTargetFlags`, `cli/cmd/build.go`)

Every `goleo build` flag is either honoured by a target or **refused** there — never
accepted and ignored. `validateTargetFlags` is the single gate, and
`cli/cmd/build_flags_test.go` asserts the full *flag x target* matrix, so a new flag that
nobody wires in surfaces as an accepted no-op instead of shipping as one. `--no-sign` is the
one deliberate exception (it asks for something *not* to happen, which a target with no
signing step satisfies); the reasoning is recorded next to its test.

Related gates, same class: **CI fails on unreachable code in `cli/`** (`deadcode` rooted at
the `goleo` binary's `main`) **and on unused unexported identifiers anywhere in `runtime/` or
`cli/`** (`staticcheck -checks U1000`, which needs no entry point and covers fields and vars,
not just funcs). Both are in `ci.yml`. Do not park a helper, a field or a build path "for
later" — Go will not warn, but the build will. Only `U1000` is enabled from staticcheck, on
purpose; SPIKES.md records why, and what the two checks found.

**Both gates run once per desktop GOOS and fail only on a finding present in ALL of them**,
because each tool analyses one GOOS at a time and this is a cross-platform CLI. Never act on
a single-GOOS "unused" result: a helper defined untagged whose only caller sits in a
`_linux.go` / `_windows.go` file looks dead on every *other* platform. Deleting
`deeplink.slug()` on exactly that reasoning broke the Linux build, and the same gate run on
CI's ubuntu runner called two live Windows helpers unreachable. If you are removing something
the gate flags, check the other platforms first.

Minimum OS versions have **one** source each (`cli/cmd/mobile_minversion.go`):
`mobile.android.min_sdk` / `mobile.ios.deployment_target` drive both gomobile
(`-androidapi`/`-iosversion`) and the native project (`minSdk`/`deploymentTarget`), with
`--android-api` / `--ios-target` as explicit per-build overrides. They must agree: a Go
library whose minimum exceeds the app's fails to link. The dev and release Android templates
are asserted to declare the same levels. **iOS will not go below 15.4, and the floor is the
lowest version the shell WORKS on rather than the lowest that builds** — the two diverge, and
every version between them yields an app that compiles, signs, launches and is missing
advertised features with nothing in the build output. iOS registers no native provider for
camera or geolocation, so both reach the hardware only through the WebView, whose permission
callbacks are `@available` at 15.0 (camera/mic) and 15.4 (geolocation);
`TestNoShellDelegateNeedsMoreThanTheIOSFloor` fails if any `@available` declaration in
`AppDelegate.swift` outruns the floor, so a new iOS-N-only delegate forces a deliberate choice
between raising the floor and writing an `if #available` fallback. Below 13.0 the older
failure still applies — the generated shell
adopts the UIScene lifecycle (`UIApplicationSceneManifest` + `SceneDelegate`), which an older
system ignores, so the app would build, sign and launch to a black screen; the version is
refused instead.

The frontend is embedded in the Go library and served over `http://127.0.0.1:<port>` on
mobile — it is **not** copied into the native project. A loopback origin is a secure context
and `file:///android_asset` is not, so the native copy could not serve the UI even if
something wanted it to; it was a second copy of the whole frontend in every APK/AAB/.app.

## Frontend Bridge (@goleo/bridge)

The public API is `bridge/src/index.ts` — read it there. What is NOT apparent from the
signatures:

- `initBridge()` selects a transport in priority order native IPC -> WebSocket -> HTTP
  and falls back transparently; `isNative()` reports which won.
- Every wrapper that has a browser equivalent degrades to it when no Go backend is
  reachable, and throws when one IS reachable but the call fails. Do not add a wrapper
  that invents a plausible value on failure — several used to, and a failed `showMessage`
  reading as the user clicking OK was a destructive-action hazard.
- Bridge lifecycle events: `bridge:connected` / `disconnected` / `reconnecting` /
  `reconnectFailed`.

## Project Template (created by goleo new)

`goleo new` then `ls` beats a tree here. The part that matters: **`backend/main.go` and
every `backend/gomobile/*.go` (`gomobile.go`, `notifier.go`, one per provider) are
GENERATED** and regenerated before every
`new`/`dev`/`build`/`emulate` run (`generateBackendEntrypoints`, `cli/cmd/generate_backend.go`),
carry a `// Code generated by goleo. DO NOT EDIT.` header, and are gitignored. All
app-specific logic lives in `backend/app/app.go` — the one file a developer edits.
`backend/init.js` is optional and drives window creation through the embedded JS engine.

## User Commands (in root package.json)

`cat package.json` in a scaffolded project for the `goleo:*` script list. The
distinction the names do not make obvious: `goleo:build*` produces a **standalone
binary**, `goleo:bundle*` produces a **native installer** (both read icon + metadata
from `goleo.json`'s `bundle` section), and `goleo:sideload-android` builds the APK then
`adb install`s it to a connected device.

## Getting Started

See README.md. (`npm install -g @goleo/cli` or `go install .../cli/goleo@latest`, then
`goleo new`, `goleo dev`, `goleo build`.)

## Dependencies

The dependency list itself is in `go.mod` — only what you cannot read there is recorded here.

- **`github.com/crgimenes/glaze`** is the sole webview binding for all three desktops, and it
  is `replace`d with the `daforester/glaze` fork **solely** for the Windows permission
  auto-grant. See "Why goleo pins a glaze fork" below before touching it.
- **`github.com/ebitengine/purego`** (webview) and **`github.com/gogpu/systray`** (tray) each
  ship a `fakecgo` shim exporting `_cgo_init`; they collide at **Mach-O** link time, which is
  why macOS has its own objc tray backend. ELF and PE tolerate the duplicate. Cross-*link* an
  executable per target, not just `go build ./...`, or this stays invisible.

`golang.org/x/mobile` is deliberately **not** a dependency of this module: the mobile build
path installs `gomobile`/`gobind` as tools and resolves x/mobile from the module cache per
build (`-mod=mod`), because its bind-support packages are not vendorable. See SPIKES.md.

### Vendoring (third-party code is committed)

All third-party Go dependencies are **vendored** (`vendor/` in the root module) and
committed, so builds never break if an upstream repo disappears — important because
some deps are pre-1.0 / single-maintainer (notably `crgimenes/glaze`). Go
automatically builds with `-mod=vendor` when `vendor/` is present; CI fails if
`vendor/` drifts from `go.mod`.

`cli/npm/goleo/` is **not** a separately-maintained module: it's a generated copy of
the root (runtime + go.mod + vendor + bridge) produced by `cli/npm/copy-source.js`
at `npm publish`/`scripts/setup.*` time and gitignored. It inherits the root's vendor
tree, so it needs no separate vendoring or pinning and CI doesn't check it.

- **Update a dep:** `scripts/update-vendor.{sh,ps1} github.com/crgimenes/glaze@v0.0.46`
  (bumps it in the root module, then re-runs `go mod tidy && go mod vendor`).
- **Update everything:** `scripts/update-vendor.{sh,ps1} -u ./...`
- **Just refresh after editing go.mod:** `scripts/update-vendor.{sh,ps1}` (no args).
- **Pin glaze to your own fork** (extra insulation): `scripts/pin-glaze-fork.{sh,ps1} github.com/<you>/glaze`.

The `spikes/` directories are separate throwaway proof modules and are intentionally
not vendored.

**The Gradle wrapper JAR is vendored too** (`cli/cmd/gradlewrapper/gradle-wrapper.jar`,
embedded and written into a generated Android project by `ensureGradleWrapper`). It used to
be fetched with `http.Get` at build time: no timeout, so a hung connection hung the build
indefinitely, and no integrity check on a JAR that is then executed via `java -classpath`.
Same reasoning as the Go deps — and it keeps the Android build path off the network. Its
version must match `gradleWrapperVersion` and the template's `distributionUrl`; a test
asserts that, and `checkWrapperJar` verifies the embed really is a wrapper JAR. See
`cli/cmd/gradlewrapper/README.md` to update it.

## Key Design Decisions

1. **WebSocket-first communication**: Persistent bidirectional connection with low latency. HTTP POST is the fallback.

2. **Embedded frontend assets**: Production builds embed the entire frontend dist into the Go binary via //go:embed, producing a single self-contained executable.

3. **Vite for frontend tooling**: Fast HMR in development, optimized builds for production. The Vite proxy config forwards API and WebSocket calls to the Go backend during dev.

4. **gomobile for mobile targets**: Uses golang.org/x/mobile (gomobile) to build an Android `.aar` and an iOS `.xcframework` from the Go backend, which are then consumed by the native shells — Gradle turns the AAR into the `app.apk`/`app.aab` you ship, so the AAR is an intermediate and is deleted.

5. **Framework-agnostic frontend**: The default template uses Vue, but any web framework works. The bridge library communicates via WebSocket/HTTP, so it can be used with React, Svelte, Angular, or vanilla JS.

6. **Cobra for CLI**: The CLI uses spf13/cobra for command structure, which is the standard for Go CLI tools.

## Platform Support

| Feature | Windows | Linux | macOS | Android | iOS | PWA |
|---------|---------|-------|-------|---------|-----|-----|
| Dev mode | yes | yes | yes | n/a | n/a | yes |
| Desktop build | yes | yes | yes | n/a | n/a | n/a |
| Mobile build | n/a | n/a | yes | yes | yes | n/a |
| PWA build | yes | yes | yes | yes | yes | yes |
| PWA dev mode | yes | yes | yes | yes | yes | yes |
| Gomobile | n/a | n/a | yes | yes | yes | n/a |

*Cross-compilation for mobile is only supported on macOS due to Apple requirements and gomobile limitations. Android can be built on any platform with the NDK, but ios requires macOS.

## WebView / Native Window

Detail: **[`docs/agents/webview.md`](docs/agents/webview.md)** — read it before touching
`runtime/webview*.go`, the glaze dependency, window modes or multi-window.

The invariants, which hold whether or not you read that file:

- **Every desktop target is cgo-free and cross-compilable from one machine.** `CGO_ENABLED=0`
  everywhere; `github.com/crgimenes/glaze` (purego) is the *only* webview binding for
  Windows/macOS/Linux. There is no cgo webview fallback left. Do not introduce a cgo
  dependency into the desktop path.
- **Do NOT drop the `replace github.com/crgimenes/glaze => github.com/daforester/glaze`
  directive** from `go.mod`, and keep `goleo new` scaffolding it. Its sole purpose is the
  Windows WebView2 permission auto-grant; without it the code still *compiles* and silently
  loses camera/mic/geolocation on Windows. It cannot live in goleo's own runtime — glaze
  exposes only the HWND, and WebView2 has no HWND-to-interface recovery.
- Additional windows are **child processes** by default because native webviews own the GUI
  thread. macOS and Linux are main-thread-only, so extra in-process windows there share the
  primary run loop — reimplementing that as per-thread loops deadlocks on macOS.

## Host features via the bridge

Detail: **[`docs/agents/host-features.md`](docs/agents/host-features.md)** — read it before
adding or changing a `runtime/<feature>/` package, a mobile provider, or the generated
backend entry points.

The invariants:

- Each feature is a `runtime/<feature>/` sub-package behind a `goleo_*` build tag, with a
  `Provider` interface so a mobile shell can register a real implementation. Unsupported
  platforms return `errors.ErrUnsupported`, never a generic error — callers branch on it to
  choose a browser fallback.
- A `Provider` is only real when **all three** pieces exist: `runtime.Set<X>Provider`, a
  `backend/gomobile/<x>.go` binding, and a call in **both** shells. Dialogs shipped with
  only the first, so every `goleo:dialog*` call on Android and iOS failed at runtime. A
  provider nobody registers is a valid program, so nothing caught it —
  `mobile_providers_test.go` now does. Any provider the shells wire unconditionally must
  also have its tag in `nativeShellProviderTags`, or apps that do not enable the feature
  fail to compile their shell (invisible in the demo scaffold, which enables everything).
- Mobile provider methods take and return **JSON strings**, not the runtime's option
  structs: gobind cannot bind a struct pointer or a `[]string` across packages in a
  reverse-bound method, and it **omits** what it cannot bind rather than failing the build.
- **`backend/main.go` and `backend/gomobile/*.go` are GENERATED** and regenerated on every
  `new`/`dev`/`build`/`emulate`. Never edit them; all app logic lives in `backend/app/app.go`.
- The Android manifest's permissions are **derived** from the `Register*` calls the scanner
  finds, plus `mobile.android.extra_permissions` — not from the compiled tag set, which is
  deliberately a superset. **Every hardware feature a declared permission IMPLIES must also
  be declared `required="false"`**, or Play filters the app off devices lacking that
  hardware. Nothing local catches that — see `cli/cmd/android_permissions.go`.

## Desktop subsystems

Detail: **[`docs/agents/desktop-subsystems.md`](docs/agents/desktop-subsystems.md)** — read it
before changing windowing/lifecycle, the native-IPC or scheme-asset transports, OS
integration, or the bundling/publishing path in `cli/cmd/`.

The invariants:

- `App.Quit()` is the single idempotent shutdown funnel. `Run` calls `runtime.LockOSThread()`
  because the native webview is thread-affine, and `StartServer` must NOT overwrite an
  existing `a.ctx` — doing so orphans `a.cancel()` and shutdown hangs. Both were real bugs.
- The tray owns the **main thread** on Windows/Linux, which is why a tray app is a headless
  controller with windows alongside it rather than a window that also has a tray.
- Native IPC and scheme assets are **opt-in** (`Config.NativeIPC`, `Config.SchemeAssets`) and
  the loopback server stays up as the fallback transport either way.
- Every `goleo build` flag is either honoured by a target or **refused** there — never
  accepted and ignored. `validateTargetFlags` is the single gate and the flag x target matrix
  is asserted in tests; a flag that silently does nothing has shipped more than once.

## Security (always applies)

- **Capability ACL** (`runtime/policy.go`): `Policy` (Allow list with `prefix*` + always-safe
  core) enforced centrally in `Bridge.HandleRequest` — deny-by-default when set, permissive when
  not. `App.SetPolicy` (takes a `*Policy`). `Allow` (method-level) **and** `FSRoots` are enforced;
  `HTTPHosts`/`ShellPrograms` are **reserved** (goleo has no http or shell plugin to gate).
- **Filesystem scope** (`runtime/fs_scope.go`, `Config.FSScope`): the `fs` plugin is confined to
  the app data dir ∪ `Policy.FSRoots` ∪ **session grants** (paths the user picked in a native
  dialog — `dialogs_reexport.go` calls `Bridge.GrantFSPath`/`AddFSRoot`, the Tauri model, so
  "pick a file then read it" needs no config). Writes/deletes outside scope are refused; reads
  warn once per path and will become errors. A deny-list (`C:\Windows`, `%ProgramFiles%`,
  `/usr`, `/etc`, …) blocks writes in *every* mode, symlinks are resolved before the check, and
  `FSScopeUnrestricted` / `GOLEO_FS_UNRESTRICTED=1` is the opt-out. Enforcement is
  `Bridge.checkFSPath`, which every handler in `fs_reexport.go` calls — **not**
  `Policy.AllowsFSPath`, whose "empty means unconstrained" rule is the opposite of the default
  the plugin needs (it is retained as a raw helper only). Before this, `validatePath` rejected
  only *relative* traversal, so `goleo:fsDelete` on an absolute path reached `os.RemoveAll` —
  and `RegisterFS` is in the default desktop bundle both scaffolds enable.
- **Server hardening** (`runtime/server.go`): production loopback-only bind, origin allow-list on
  the WS upgrade **and `/api/invoke`** + CORS, per-launch token injected into `index.html`
  (generation fails closed; comparison is constant-time). Dev mode is permissive only about
  *which* origin serves the frontend — loopback, private-network and link-local origins are
  allowed on any port (so Vite, `goleo emulate android` via 10.0.2.2, and LAN device testing all
  work), public origins are rejected, and `GOLEO_DEV_ALLOWED_ORIGINS` is the escape hatch. It used
  to allow *any* origin in dev, which let any page the user visited drive the whole bridge.
  Native IPC (above) sidesteps this surface entirely for the window that uses it — no WS upgrade,
  no token needed — while `Policy` still gates every call.
- **The mobile WebView's camera/mic/geolocation prompts are origin-gated** (`isAppOrigin` in
  both Android shells, `GoleoWebPermissionDelegate.isAppOrigin` on iOS): the parsed **host**
  must equal a loopback name (plus `10.0.2.2` in the Android *dev* shell only). Nothing in
  either shell restricts navigation, so these gates answer for whatever page the WebView
  reaches. iOS granted all three unconditionally; Android matched a string *prefix*, which
  also accepts `http://127.0.0.1.evil.com`; and Android's geolocation callback had no check
  at all. Compare hosts, never prefixes — `devOriginAllowed` in `runtime/server.go` is the
  reference shape.
- **`goleo:openURL` is scheme-allow-listed** (`runtime/platform.go`): `http`/`https`/`mailto`/`tel`
  plus the app's own `Config.URLScheme` (registered via `AllowURLScheme`). `file://`, UNC paths and
  bare filesystem paths are refused — the OS handlers would otherwise open executables.
