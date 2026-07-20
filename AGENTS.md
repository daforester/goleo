# Goleo Framework — AI Context File

## Overview

Goleo is a Go-based framework for building cross-platform desktop and mobile applications using Go for the backend and web technologies for the frontend. It supports **Windows, Linux, macOS, Android, and iOS** from a single codebase.

**Core concept**: Write your app logic in Go, build your UI with any web framework (Vue, React, Svelte, vanilla JS, etc.), and Goleo handles the bundling, communication bridge, and platform-specific packaging.

## Repository Structure

```
goleo/
├── AGENTS.md              # This file - AI context
├── README.md              # Project overview
├── go.mod                 # Root Go module
├── runtime/               # Go runtime library (imported by user apps)
│   ├── app.go             # App lifecycle, config, run loop
│   ├── bridge.go          # Frontend-backend communication bridge
│   ├── server.go          # HTTP/WS server, CORS, API endpoints
│   ├── websocket.go       # WebSocket client management, hub pattern
│   ├── platform.go        # OS/platform detection utilities
│   ├── examples.go        # Built-in sample commands
│   └── embed.go           # Embed.FS helper for frontend assets
├── cli/                   # CLI tool (goleo binary)
│   ├── goleo/main.go      # Entry point (go install .../cli/goleo@latest)
│   └── cmd/
│       ├── root.go        # Root cobra command
│       ├── new.go         # goleo new - scaffold a project
│       ├── templates.go   # All templates for goleo new
│       ├── dev.go         # goleo dev - dev mode with hot reload
│       ├── build.go       # goleo build - build for platforms
│       └── version.go     # goleo version
├── bridge/                # npm package: @goleo/bridge
│   ├── package.json
│   ├── tsconfig.json
│   ├── README.md
│   └── src/
│       ├── index.ts       # Public API exports
│       ├── bridge.ts      # WebSocket/HTTP bridge implementation
│       └── types.ts       # TypeScript type definitions
├── templates/app/         # User project templates (consumed by goleo new)
├── docs/
│   └── guide/             # Developer Guide (multi-page): install, setup, build,
│                          # packaging/icons, deploy, wiring, RPC, menus, tray, mobile
└── scripts/               # Utility scripts
```

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

1. Frontend is built with Vite into rontend/dist/
2. The dist/ directory is embedded into Go binary via //go:embed
3. Go binary serves embedded static files along with API on the same port
4. A single self-contained executable is produced

## Go Runtime Library (untime/)

The runtime package is imported by user applications.

### App Lifecycle

`go
app := runtime.New(runtime.Config{
    Title:      "My App",
    Width:      1024,
    Height:     768,
    DevMode:    false,
    Port:       0,       // 0 = random available port
    EmbedFS:    frontendFS,
    OnStartup:  func(ctx context.Context) { },
    OnShutdown: func(ctx context.Context) { },
})
runtime.RegisterBuiltins(app.Bridge())
app.Bridge().Handle("myCommand", myHandler)
app.Run() // blocks until SIGINT/SIGTERM
`

### Config Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| Title | string | "Goleo App" | Window title |
| Width | int | 1024 | Window width |
| Height | int | 768 | Window height |
| DevMode | bool | false | Enable dev mode (CORS, no embedded files) |
| DevServer | string | "" | Frontend dev server URL |
| Port | int | 9842 | Server port (0 = random) |
| WindowMode | WindowMode | WindowModeBrowser | Display mode (browser/webview/mobile) |
| EmbedFS | any | nil | Embedded frontend filesystem |
| OnStartup | func(ctx) | nil | Startup callback |
| OnShutdown | func(ctx) | nil | Shutdown callback |

### Bridge API (Go side)

#### Registering commands

`go
app.Bridge().Handle("add", func(ctx context.Context, args json.RawMessage) (any, error) {
    var params map[string]int
    json.Unmarshal(args, &params)
    sum := params["a"] + params["b"]
    return map[string]int{"sum": sum}, nil
})
`

#### Built-in commands

| Command | Args | Returns | Description |
|---------|------|---------|-------------|
| goleo:getOS | none | OSInfo | OS name, arch, version |
| goleo:getPlatform | none | PlatformInfo | Platform type (desktop/mobile) |
| goleo:getArch | none | string | CPU architecture |
| goleo:getEnv | {key} | string | Get whitelisted env var |
| goleo:openURL | {url} | void | Open URL in browser |
| goleo:showMessage | {title, message} | void | Log a message |

#### Events (Go side)

`go
// Emit events to frontend
app.Emit("data:updated", map[string]any{"count": 42})

// Listen for events from frontend
app.On("app:ready", func(ctx context.Context, data json.RawMessage) {
    log.Print("Frontend is ready")
})
`

### Server Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| /api/invoke | POST | Call a Go function (HTTP fallback) |
| /ws | GET | WebSocket upgrade for real-time communication |
| /api/health | GET | Health check |
| / | GET | Static file serving (production only) |

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
| goleo build android | Build an installable Android .apk (gomobile AAR + Gradle) |
| goleo build ios | Build iOS .xcframework via gomobile |
| goleo build pwa | Build Progressive Web App (no Go backend) |
| goleo build --bundle | Also package the desktop app into a native installer (dist/bundle/) |
| goleo build --publish | Also write an ed25519-signed update manifest (needs GOLEO_UPDATE_PRIVKEY) |
| goleo emulate android | Run in dev mode on a connected Android device or emulator |
| goleo install android | Sideload the built app.apk onto a connected device + launch it |
| goleo generate types | Generate frontend/src/goleo.d.ts (typed invoke() overloads) |
| goleo generate updater-key | Generate an ed25519 keypair for signing update manifests |
| goleo version | Print version |

### Build Targets

| Target | GOOS | GOARCH | Output | Dependency |
|--------|------|--------|--------|------------|
| current | auto | auto | binary | none |
| windows | windows | amd64 | .exe | none |
| linux | linux | amd64 | binary | none |
| darwin | darwin | amd64 | binary | none |
| android | android | arm64 | .aar | gomobile + NDK |
| ios | ios | arm64 | .xcframework | gomobile + Xcode |
| pwa | js | wasm | dist-pwa/ | none |

## Frontend Bridge (@goleo/bridge)

### Initialization

`	ypescript
import { initBridge, invoke, on, getOSInfo } from '@goleo/bridge'

await initBridge({
  serverUrl: 'http://localhost:9842',
  wsUrl: 'ws://localhost:9842/ws',
  autoReconnect: true,
})
`

### API Reference

| Function | Returns | Description |
|----------|---------|-------------|
| initBridge(config) | Promise<void> | Initialize bridge and connect |
| invoke(method, args) | Promise<T> | Call a Go backend function |
| on(event, callback) | () => void | Listen for events (returns unsubscribe) |
| off(event, callback) | void | Remove event listener |
| getOSInfo() | Promise<OSInfo> | Get OS information |
| getPlatformInfo() | Promise<PlatformInfo> | Get platform info |
| getArch() | Promise<string> | Get CPU architecture |
| getEnv(key) | Promise<string> | Get environment variable |
| openURL(url) | Promise<void> | Open URL in browser |
| disconnect() | void | Disconnect from backend |
| isConnected() | boolean | Check connection status |

### Events

| Event | Payload | Description |
|-------|---------|-------------|
| bridge:connected | {} | WebSocket connected |
| bridge:disconnected | {} | WebSocket disconnected |
| bridge:reconnecting | {attempt: number} | Attempting reconnect |
| bridge:reconnectFailed | {} | Max reconnection attempts reached |

## Project Template (created by goleo new)

`
my-app/
├── .gitignore              # Ignores generated backend files + build output
├── goleo.json              # Goleo project configuration
├── go.mod                  # Go module
├── package.json            # Root with goleo:* scripts
├── backend/
│   ├── app/
│   │   └── app.go          # All startup logic — the file you actually edit
│   ├── commands/
│   │   └── commands.go     # User-defined backend commands
│   ├── gomobile/
│   │   ├── gomobile.go     # GENERATED — calls app.New; do not edit, gitignored
│   │   └── notifier.go     # GENERATED — do not edit, gitignored
│   ├── init.js             # Optional JS startup script (window creation)
│   └── main.go             # GENERATED — calls app.New; do not edit, gitignored
└── frontend/
    ├── package.json        # Frontend deps (Vue + Vite + bridge)
    ├── index.html
    ├── vite.config.ts      # Vite config with API proxy
    ├── tsconfig.json
    ├── env.d.ts
    └── src/
        ├── main.ts         # Frontend entry, inits bridge
        ├── App.vue         # Root Vue component
        └── style.css
`

## User Commands (in root package.json)

`json
{
  "scripts": {
    "goleo:dev": "goleo dev",                                  // desktop dev (Go + Vite HMR)
    "goleo:dev-pwa": "goleo dev pwa",                          // PWA dev (Vite only)
    "goleo:dev-android": "goleo emulate android",              // dev on a connected device / emulator

    "goleo:build": "goleo build",                             // standalone binary (current OS)
    "goleo:build-windows": "goleo build windows",
    "goleo:build-linux": "goleo build linux",
    "goleo:build-darwin": "goleo build darwin",
    "goleo:build-android": "goleo build android",              // installable app.apk
    "goleo:build-ios": "goleo build ios",
    "goleo:build-pwa": "goleo build pwa",

    "goleo:bundle": "goleo build --bundle",                   // native installer (current OS)
    "goleo:bundle-windows": "goleo build windows --bundle",   // NSIS .exe installer
    "goleo:bundle-linux": "goleo build linux --bundle",       // .deb / .rpm
    "goleo:bundle-darwin": "goleo build darwin --bundle",     // .app + .dmg

    "goleo:publish": "goleo build --publish",                 // signed update manifest
    "goleo:sideload-android": "goleo build android && goleo install android",
    "goleo:types": "goleo generate types"                     // typed bridge d.ts
  }
}

Standalone binaries come from `goleo:build*`; native installers from `goleo:bundle*`
(both read icon + metadata from `goleo.json`'s `bundle` section — see the Packaging
guide). `goleo:sideload-android` builds the APK then `adb install`s it to a connected
device.
`

## Getting Started

`ash
# Install the CLI (npm, or `go install github.com/daforester/goleo/cli/goleo@latest`)
npm install -g @goleo/cli

# Scaffold a project (or, no global install: npx @goleo/cli new my-app)
goleo new my-app
cd my-app
cd frontend && npm install && cd ..
goleo dev        # Start development
goleo build      # Build for current platform
`

## Dependencies

### Go Dependencies
- github.com/spf13/cobra - CLI framework
- github.com/gorilla/websocket - WebSocket support
- github.com/crgimenes/glaze - cgo-free WKWebView/WebKitGTK/WebView2 backend (the sole default webview binding for ALL desktops; pinned to the daforester/glaze fork)
- github.com/webview/webview_go - cgo WKWebView/WebKitGTK (legacy fallback, `goleo_cgo_webview`)
- github.com/ebitengine/purego - dlopen/FFI used by the cgo-free webview backends
- github.com/gogpu/systray - cgo-free system tray
- github.com/dop251/goja - JS engine for `init.js`
- golang.org/x/sys / golang.org/x/mobile - platform + gomobile support

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

- **Update a dep:** `scripts/update-vendor.{sh,ps1} github.com/crgimenes/glaze@v0.0.32`
  (bumps it in the root module, then re-runs `go mod tidy && go mod vendor`).
- **Update everything:** `scripts/update-vendor.{sh,ps1} -u ./...`
- **Just refresh after editing go.mod:** `scripts/update-vendor.{sh,ps1}` (no args).
- **Pin glaze to your own fork** (extra insulation): `scripts/pin-glaze-fork.{sh,ps1} github.com/<you>/glaze`.

The `spikes/` directories are separate throwaway proof modules and are intentionally
not vendored.

### npm Dependencies (bridge)
- 	ypescript - Build tool

### User Frontend Dependencies (template)
- ue - UI framework (default, swappable)
- ite - Build tool and dev server
- @goleo/bridge - Frontend-backend bridge
- @vitejs/plugin-vue - Vite Vue plugin

## Key Design Decisions

1. **WebSocket-first communication**: Persistent bidirectional connection with low latency. HTTP POST is the fallback.

2. **Embedded frontend assets**: Production builds embed the entire frontend dist into the Go binary via //go:embed, producing a single self-contained executable.

3. **Vite for frontend tooling**: Fast HMR in development, optimized builds for production. The Vite proxy config forwards API and WebSocket calls to the Go backend during dev.

4. **gomobile for mobile targets**: Uses golang.org/x/mobile (gomobile) to build Android .aar and iOS .xcframework artifacts from the Go backend code.

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

*Cross-compilation for mobile is only supported on macOS due to Apple requirements and gomobile limitations. Android .aar can be built on any platform with the NDK, but ios requires macOS.

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
- **Opt-in fallback (one release, then removed):**
  - **macOS/Linux cgo webview_go (`-tags goleo_cgo_webview`):** `runtime/webview.go`
    (`github.com/webview/webview_go`, cgo WebKitGTK/WKWebView); `goleo build` selects it
    (with `CGO_ENABLED=1`) when `GOLEO_CGO_WEBVIEW=1`. Note: a `CGO_ENABLED=0` Linux
    build also excludes `runtime/camera`'s cgo V4L2 impl — hence `camera_linux.go` is
    `cgo`-tagged with a pure-Go stub fallback.

So **every desktop target is pure-Go and cross-compilable from one machine**, on a
single binding. Shutdown unblocks the run loop via a per-backend `endRunLoop()`
(glaze/cgo `Terminate()`) — not a GOOS check.

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
frontend's browser-API fallbacks resolve instead of hanging. The real
implementation is WebKitGTK-specific on Linux (`webview_permissions_linux.go`);
it is a no-op elsewhere (`webview_permissions_other.go`).

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

## Session Summary (Jul 8, 2026)

### PWA Build Target
- Added `goleo build pwa` — builds PWA (no Go backend, frontend only to `dist-pwa/`)
- Added `goleo dev pwa` — starts Vite dev server without Go backend
- Sets `VITE_GOLEO_PLATFORM=pwa` env var for both dev and build

### Bridge Graceful Degradation
- Bridge now handles connection timeout → local-only mode (no backend fallback)
- `backend` config option for explicit platform targeting (desktop/mobile/pwa)
- `showNotification`, `showAlert`, etc. fall back to browser Notification API
- `getOSInfo`, `getPlatformInfo`, `getArch`, `getEnv`, `openURL` fall back to browser APIs when Go backend unavailable

### init.js Restored
- `init.js` is a *feature*, not stale — gives JS developers control over window creation
- Back in `tmplInitJS` in `templates.go`, `new.go` files map, and `create-app.ts` files map
- Embedded via `//go:embed init.js` in main.go template alongside `//go:embed all:frontend/dist`

### Template Cleanup
- Removed stale entries from `create-app.ts`: `commands/commands.go`, `commands/init.js`
- Files are at `backend/commands.go` and `backend/init.js` (flat, not in a `commands/` subdir)

### Host Features via Bridge
- Architectured a permission-gated host features system (like Tauri/Electron capabilities)
- Each feature is a `runtime/<feature>/` sub-package with platform-specific implementations behind build tags
- Desktop features split from mobile via `runtime/desktop.go` (`//go:build !android && !ios`) calling `RegisterClipboard`, etc.
- `RegisterBuiltins()` reduced to core-only (OS info, env, openURL, notifications); `RegisterDesktopFeatures()` for desktop extras
- Mobile-only features use `goleo_*` build tags (e.g. `goleo_nfc`, `goleo_ble`) so Android manifest only declares what's actually used
- `cli/cmd/scan.go` — source scanner that detects `runtime.Register*()` calls and emits the corresponding build tags + manifest entries
- `runtime/clipboard/` — implemented feature with read/write text via platform shell commands; re-exported via `runtime/clipboard_reexport.go`
- `runtime/dialogs/` — native dialogs (file open/save, folder picker, message box, input prompt) via PowerShell (Windows), osascript (macOS), zenity (Linux)
- `runtime/fs/` — file system access (read/write text+binary, list dir, delete, app/home dirs) with path traversal protection
- `runtime/geolocation/` — geolocation via Go backend (stub on desktop, needs `goleo_geolocation` tag on mobile) with full browser API fallback
- `bridge/src/clipboard.ts`, `dialogs.ts`, `fs.ts`, `geolocation.ts` — TS convenience wrappers with browser API fallbacks, all exported from `@goleo/bridge`
- `cli/cmd/generate.go` — `goleo generate types` command that generates `frontend/src/goleo.d.ts` with typed `invoke()` overloads for all 48+ built-in commands

### Complete Host Feature Set (13 features)
All 13 features implemented with Go sub-packages + re-export bridge handlers + TS convenience wrappers with browser API fallbacks:

Every feature package now exposes a `Provider` interface + `SetProvider`/`runtime.Set<Feature>Provider`, so a mobile shell (or a future native backend) can register a real implementation instead of relying on the `_mobile.go` "no provider registered" error. Desktop status below is the *built-in Go implementation*, not just "compiles":

| Feature | Go Pkg | Build Tag | Desktop | Mobile | TS Browser Fallback |
|---------|--------|-----------|---------|--------|---------------------|
| **Core (9)** | `runtime/` (builtins) | — | Native | Provider | navigator/Notification |
| **Clipboard** | `runtime/clipboard/` | `goleo_clipboard` | Native (PowerShell/pbcopy/xclip) | Provider | `navigator.clipboard` |
| **Dialogs** | `runtime/dialogs/` | `goleo_dialog` | Native (PowerShell/osascript/zenity) | Provider | `<input type="file">` |
| **FileSystem** | `runtime/fs/` | `goleo_fs` | Native | Provider | Requires Go |
| **Geolocation** | `runtime/geolocation/` | `goleo_geolocation` | Native on Windows (WinRT Geolocator) and macOS (CoreLocationCLI, opt-in); unsupported on Linux | Provider | `navigator.geolocation` |
| **Battery** | `runtime/battery/` | `goleo_battery` | Native (Win32 API / `/sys/class/power_supply` / `pmset`) | Provider | `navigator.getBattery()` |
| **WakeLock** | `runtime/wakelock/` | `goleo_wakelock` | Native (`SetThreadExecutionState` / `caffeinate` / `systemd-inhibit`) | Provider | `navigator.wakeLock` |
| **Vibration** | `runtime/vibration/` | `goleo_vibration` | Unsupported (no desktop vibrator) | Provider | `navigator.vibrate()` |
| **Sensors** | `runtime/sensors/` | `goleo_sensors` | Unsupported (no portable desktop sensor API) | Provider | Generic Sensor API |
| **Camera** | `runtime/camera/` | `goleo_camera` | Unsupported — intentionally routes to WebView `getUserMedia` | Provider | `getUserMedia` + canvas |
| **Bluetooth** | `runtime/bluetooth/` | `goleo_ble` | Unsupported — intentionally routes to Web Bluetooth | Provider | Web Bluetooth API |
| **NFC** | `runtime/nfc/` | `goleo_nfc` | Unsupported (no desktop NFC hardware path) | Provider | Web NFC API |
| **Background** | `runtime/background/` | `goleo_background` | Unsupported — desktop process runs continuously, no OS scheduler needed | Provider | Service Worker Sync |
| **Push** | `runtime/push/` | `goleo_push` | Unsupported — use the app's own WebSocket channel instead | Provider | Push API + Service Worker |

"Unsupported" packages return `fmt.Errorf("...: %w", errors.ErrUnsupported)` rather than a generic error, so callers can `errors.Is(err, errors.ErrUnsupported)` to detect "no native path on this platform, use the fallback" instead of a real failure. On Android, the Android WebView (`cli/cmd/templates/{android,android-dev}/.../MainActivity.java`) now wires `WebChromeClient.onPermissionRequest` (camera/mic) and `onGeolocationPermissionsShowPrompt` to runtime permission requests, so the getUserMedia/geolocation browser fallbacks actually work instead of silently failing; on iOS, `AppDelegate.swift` sets a `WKUIDelegate` that grants the equivalent WKWebView permission callbacks, and `Info.plist` declares the required `NS*UsageDescription` strings.

### Fully Generated Backend Entry Points
`backend/main.go` (desktop) and `backend/gomobile/{gomobile.go,notifier.go}` (mobile) are no longer scaffolded once and left as editable source — they're pure boilerplate (call `app.New(...)`, nothing app-specific) regenerated fresh by `generateBackendEntrypoints()` (`cli/cmd/generate_backend.go`) before every `goleo new`/`dev`/`build`/`emulate` run, exactly like the Android/iOS shell templates under `cli/cmd/templates/`. All app-specific logic — commands, feature wiring, `Width`/`Height`/`Port`/`Title` — lives entirely in `backend/app/app.go`, the one file a developer edits. Each generated file carries a `// Code generated by goleo. DO NOT EDIT.` header. A new `.gitignore` (`tmplGitignore` in `templates.go`, mirrored in `create-app.ts`) excludes the three generated files plus `.goleo/`, build outputs, and `node_modules` — none of that previously had a `.gitignore` at all. `backendPkgDir()` (`build.go`) now detects the `backend/` layout by checking for the directory itself rather than `backend/main.go`, since that file may not exist yet on a fresh clone before the first CLI run regenerates it. A new `parseModuleName()` helper (`replace.go`) reads the module path out of `go.mod` at CLI runtime so these files can be rendered outside of `goleo new` (where `projectConfig.ModuleName` was previously only ever constructed once, from the CLI arg).

## Desktop subsystems (windowing, lifecycle, distribution, security)

Added on top of the core bridge/feature system. Full rationale + status in
`docs/roadmap.md` (the masterplan); feasibility findings in `SPIKES.md`.

### Windowing
- **Multi-process (default, cross-platform):** `runtime/windowmanager.go` `WindowManager` spawns
  each extra window as a child process (`runtime/window_child.go`, `GOLEO_WINDOW=1`) that hosts
  one webview against the shared server. The primary window is hosted in-process by `runWebview`.
- **In-process (Windows, opt-in):** `inProcWindowManager` hosts each window on its own
  `LockOSThread` goroutine (proven in `spikes/win-multiwindow/`). Selected by
  `Config.InProcessWindows` on Windows. Both implement the `windowSpawner` interface.
- API: `App.OpenWindow/CloseWindow/ListWindows`, bridge `goleo:window{Open,Close,List}`,
  `bridge/src/window.ts`; `WindowOptions.ExitOnClose` quits the app when that window closes.

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
  added to all `WebviewWindow` backends (`webview_windows.go`, `webview.go`, `webview_stub.go`).
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
  (`https://goleo.localhost`) via `spikes/goleo-scheme-verify`. **Requires the glaze fork** (`NewWithOptions`): goleo pins
  `crgimenes/glaze => daforester/glaze` via `replace`, and because Go `replace` directives don't
  transit, **any downstream module importing goleo's runtime needs the same replace** — `goleo new`
  scaffolds it into the generated `go.mod`. See `SPIKES.md` (2026-07-13). Update: the scheme API is
  now **merged into upstream `crgimenes/glaze`**; the fork (currently `v0.0.32-goleo.5`) is retained
  only until upstream cuts a release carrying it *and* the Windows permission auto-grant moves off
  the fork — see the fork-retirement note in `SPIKES.md` + `spikes/glaze-scheme-secure/PERMISSION_HOOK_ISSUE.md`.

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

### Security
- **Capability ACL** (`runtime/policy.go`): `Policy` (Allow list with `prefix*` + always-safe
  core) enforced centrally in `Bridge.HandleRequest` — deny-by-default when set, permissive when
  not. `App.SetPolicy`. Scope helpers `AllowsFSPath/AllowsHTTPHost/AllowsShellProgram`.
- **Server hardening** (`runtime/server.go`): production loopback-only bind, origin allow-list on
  the WS upgrade + CORS (dev/emulation permissive), per-launch token injected into `index.html`.
  Native IPC (above) sidesteps this surface entirely for the window that uses it — no WS upgrade,
  no token needed — while `Policy` still gates every call.

### Distribution (CLI, `cli/cmd/`)
- `bundle.go` — `goleo build --bundle`: NSIS (Windows, auto-installs `makensis` via winget/choco/
  scoop), `.app`+`.dmg` (macOS), `.deb`/`.rpm` (nfpm, Linux — with a generated hicolor icon +
  `.desktop` entry). `signing.go` — env-driven Authenticode / codesign+notarytool.
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

## Session Summary (Jul 16, 2026) — icons + Android validated end-to-end

- **App-icon generation (all platforms), pure Go.** New `cli/cmd/icons.go` (+ `icons_test.go`)
  turns a single `bundle.icon` PNG into every platform artifact; wired into `winres.go` (Windows
  exe, now multi-size), `bundle.go` (macOS `.icns`, Linux hicolor PNG + generated `.desktop`), and
  the mobile build path. See Distribution → Icons above and `docs/guide/04-packaging-icons.md`.
- **Mobile launcher icons.** Android `mipmap-*/ic_launcher(+_round).png` (all densities, round via
  a circular alpha mask) referenced from a `{{if .HasIcon}}`-gated `android:icon` in the manifest;
  iOS `AppIcon.appiconset` gated by `ASSETCATALOG_COMPILER_APPICON_NAME` in `xcodegen.yml`.
- **Fixed: mobile builds broke for vendored projects and for any app not enabling every feature.**
  Two real bugs, both in `SPIKES.md`:
  1. A scaffolded project commits `vendor/` → Go picks `-mod=vendor`, but `gomobile bind` needs
     `golang.org/x/mobile` bind-support packages absent from `vendor/`, and `go get -tool` refuses
     to run under vendor mode. Mobile build path now forces `GOFLAGS=-mod=mod` (`modModEnv`/
     `goToolEnv`/`setMobileEnv` in `cli/cmd/gotools.go`,`build.go`).
  2. The native shell (`MainActivity.java`/`AppDelegate.swift`) unconditionally wires all 8 native
     providers, but their gomobile bindings were gated behind per-feature `goleo_*` tags only
     enabled when the app called `Register*` — so the default scaffold failed with `cannot find
     symbol gomobile.BatteryProvider`. `mobileBindTags` (`scan.go`) now always binds
     `nativeShellProviderTags` (battery/wakelock/sensors/background/nfc/ble/clipboard/share);
     per-feature *bridge-command registration* stays opt-in via `Register*`.
- **Verified on a real Android emulator (API 36, x86_64):** `goleo build android` on a freshly
  scaffolded+vendored project produced a 66 MB APK (4 ABIs, all launcher-icon densities, all demo
  permissions), installed and launched; the Go backend + bridge ran (invoke + push events), and
  **native providers round-tripped live** — battery (real level), clipboard (write→read), FS
  (write→read via `SetHomeDir`), wake-lock, sensors (confirmed registered with the OS
  SensorManager), share. Camera/BLE/NFC need real peripherals (not on the emulator) but their
  bindings compile + are wired and permissions are present. NSIS bundle verified end-to-end on
  Windows (real installer, no path doubling) via `nsis_integration_test.go`.
