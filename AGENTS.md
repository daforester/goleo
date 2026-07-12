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
├── create-goleo-app/      # npm create goleo-app scaffold package
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
| goleo new <name> | Scaffold a new Goleo project |
| goleo dev | Start development mode (Go + Vite with HMR) |
| goleo dev pwa | Start PWA development mode (Vite only, no Go backend) |
| goleo build | Build for current platform |
| goleo build windows | Cross-compile for Windows amd64 |
| goleo build linux | Cross-compile for Linux amd64 |
| goleo build darwin | Cross-compile for macOS amd64 |
| goleo build android | Build Android .aar via gomobile |
| goleo build ios | Build iOS .xcframework via gomobile |
| goleo build pwa | Build Progressive Web App (no Go backend) |
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
    "goleo:dev": "goleo dev",
    "goleo:dev-pwa": "goleo dev pwa",
    "goleo:build": "goleo build",
    "goleo:build-windows": "goleo build windows",
    "goleo:build-linux": "goleo build linux",
    "goleo:build-darwin": "goleo build darwin",
    "goleo:build-android": "goleo build android",
    "goleo:build-ios": "goleo build ios",
    "goleo:build-pwa": "goleo build pwa"
  }
}
`

## Getting Started

`ash
# Install the CLI
go install github.com/daforester/goleo/cli/goleo@latest

# Or use npm scaffold
npm create goleo-app@latest my-app
cd my-app
cd frontend && npm install && cd ..
goleo dev        # Start development
goleo build      # Build for current platform
`

## Dependencies

### Go Dependencies
- github.com/spf13/cobra - CLI framework
- github.com/gorilla/websocket - WebSocket support
- golang.org/x/mobile - Mobile platform support (gomobile)

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

Goleo renders the desktop frontend in a **native OS webview**, with a
per-platform backend selected by build tag:
- **Windows: WebView2 (Edge Chromium)** via `github.com/jchv/go-webview2`
  (`runtime/webview_windows.go`) — **cgo-free** (COM/syscall), so Windows builds
  and cross-compiles with `CGO_ENABLED=0`.
- **Linux: WebKitGTK** / **macOS: WKWebView** via `github.com/webview/webview_go`
  (`runtime/webview.go`) — these link the system webview through **cgo**, so
  `buildForDesktop` sets `CGO_ENABLED=1` for those targets and they must be built
  on their own OS. (Dropping their cgo requirement via purego is roadmapped in
  `docs/roadmap.md`.)

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
- **Tray:** `Config.Tray` (`TrayConfig`/`TrayItem`) via `github.com/gogpu/systray` (cgo-free);
  `runtime/tray_desktop.go` (build tag `!mobilebuild && !js`), `tray_stub.go` otherwise.

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
    (`goleo://`) asset serving is **deferred to the purego milestone** — the current bindings don't
    expose a scheme handler cleanly (Windows-only edge rewrite otherwise); the purego backends add
    it uniformly. See `SPIKES.md` (2026-07-12 finding).
  - **Verified** on real WebView2 (Windows, cgo-free): a two-window app where each window completes
    an independent bidirectional round-trip over its own native channel, incl. `goleo:windowOpen`
    over native IPC, then a clean `Quit`. Also `runtime/nativeipc_test.go` (round-trip, policy,
    events, ping, pump-stop) + `bridge` tsc.

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
- `bundle.go` — `goleo build --bundle`: NSIS (Windows), `.app`+`.dmg` (macOS), `.deb`/`.rpm`
  (nfpm, Linux). `signing.go` — env-driven Authenticode / codesign+notarytool.
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
