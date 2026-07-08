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
│   ├── main.go            # Entry point
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

The frontend (browser/webview) communicates with the Go backend via WebSocket (preferred) or HTTP POST (fallback).

- **WebSocket** (preferred): Persistent bidirectional connection. Used in both dev and production. Low latency, supports server push events.
- **HTTP POST** (fallback): Calls /api/invoke endpoint when WebSocket is unavailable. No event push support.

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
├── goleo.json              # Goleo project configuration
├── package.json            # Root with goleo:* scripts
├── backend/
│   ├── go.mod              # Go module
│   ├── main.go             # App entry point with embed
│   └── commands.go         # User-defined backend commands
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

Currently, Goleo runs the frontend in a web browser (browser mode). Future versions will integrate native webview libraries for desktop:
- Windows: WebView2 (Edge Chromium)
- Linux: WebKitGTK
- macOS: WKWebView

Mobile platforms use the platform's built-in WebView component via gomobile bindings.

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
