# Goleo

Build cross-platform desktop and mobile applications with **Go** + **Web technologies**.

Write your app logic in Go. Build your UI with Vue, React, Svelte, or vanilla JS. Ship to Windows, Linux, macOS, Android, and iOS — all from a single codebase.

## Features

- **Cross-platform**: Windows, Linux, macOS, Android, iOS
- **Go backend**: Full power of Go for app logic, concurrency, system access
- **Web frontend**: Use any web framework (Vue, React, Svelte, etc.)
- **Live development**: Hot module replacement for the frontend
- **Single binary output**: Embed the frontend build into the Go executable
- **Bidirectional bridge**: Seamless WebSocket communication between frontend and backend

## Quick Start

```bash
# Install CLI
go install github.com/daforester/goleo/cli/goleo@latest

# Or scaffold via npm
npm create goleo-app@latest my-app
cd my-app
cd frontend && npm install && cd ..

# Development
goleo dev

# Build for current platform
goleo build
```

## Commands

| Command | Description |
|---------|-------------|
| `goleo new <name>` | Create a new project |
| `goleo dev` | Start dev mode (HMR for frontend) |
| `goleo build` | Build for current platform |
| `goleo build windows` | Cross-compile for Windows |
| `goleo build linux` | Cross-compile for Linux |
| `goleo build darwin` | Cross-compile for macOS |
| `goleo build android` | Build Android .aar (requires gomobile) |
| `goleo build ios` | Build iOS .xcframework (macOS only) |

## Architecture

```
┌─────────────────────┐     WebSocket     ┌──────────────────┐
│   Frontend (Web)    │ ◄───────────────► │   Go Backend     │
│   Vue/React/Svelte  │    or HTTP POST   │   + Bridge API   │
│   @goleo/bridge     │                   │   + User Commands│
└─────────────────────┘                   └──────────────────┘
```

- Frontend communicates with Go backend via WebSocket (preferred) or HTTP POST
- In production, frontend is embedded in the Go binary via `//go:embed`
- In development, Vite dev server runs with HMR, proxying API calls to Go
- Mobile builds use `gomobile` for Android (.aar) and iOS (.xcframework)
- Desktop builds open a native OS webview window (WebView2 / WebKitGTK / WKWebView); browser and PWA modes serve the same UI with no native window

## License

MIT
