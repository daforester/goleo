# Goleo Developer Guide

Build cross-platform desktop **and** mobile apps with a Go backend and a web
(HTML/CSS/JS) frontend — one codebase for **Windows, macOS, Linux, Android, iOS**,
and a PWA. Your app logic lives in Go; your UI is any web framework (Vue, React,
Svelte, vanilla…); Goleo handles the native window, the frontend↔backend bridge,
and platform packaging.

## How Goleo fits together

```
┌─────────────────────────────┐        ┌──────────────────────────────┐
│  Frontend (web UI)          │        │  Go backend (your app logic) │
│  Vue / React / Svelte / JS  │◄──────►│  runtime.App + your commands │
│  @goleo/bridge: invoke/on   │ bridge │  Bridge.Handle / Emit / On   │
└─────────────────────────────┘        └──────────────────────────────┘
        rendered in a native OS webview (or a mobile WebView, or a browser/PWA)
```

- **Desktop**: a native OS webview (WKWebView / WebKitGTK / WebView2) via one
  cgo-free binding — every desktop builds `CGO_ENABLED=0` and cross-compiles.
- **Mobile**: a native Android/iOS shell hosts the platform WebView; the Go
  backend runs in-process via gomobile.
- **PWA**: the frontend alone, no Go backend.

## The pages

1. [Installation](01-installation.md) — install the CLI + prerequisites per platform.
2. [Project setup](02-setup.md) — scaffold a project, structure, first dev run.
3. [Building](03-building.md) — standalone binaries, PWA, mobile, cross-compilation.
4. [Packaging, icons & metadata](04-packaging-icons.md) — installers, app icon, version info, signing.
5. [Deploying & updating](05-deploy.md) — distribute installers, auto-update, sideload.
6. [Wiring up your app](06-wiring-apps.md) — `Config`, commands, events, the bridge, host features.
7. [RPC in depth](07-rpc.md) — Go handlers, `invoke`, events, transports, the security policy.
8. [Native menus](08-menus.md) — the menu bar, roles, accelerators, the bridge menu API.
9. [System tray](09-systray.md) — tray icon + menu, background apps.
10. [Mobile](10-mobile.md) — dev on a real device, sideloading, permissions, providers.
11. [Host features tour](11-host-features.md) — notifications, dialogs, clipboard, filesystem, geolocation, battery, camera, Bluetooth, NFC, share, store & more.

## The 60-second version

```bash
# install the CLI
go install github.com/daforester/goleo/cli/goleo@latest

# scaffold, install frontend deps, run
goleo new my-app
cd my-app/frontend && npm install && cd ..
npm run goleo:dev            # desktop dev with hot reload

# ship it
npm run goleo:build          # standalone binary for this OS
npm run goleo:bundle         # native installer for this OS
```

New to Goleo? Start with [Installation](01-installation.md) → [Setup](02-setup.md)
→ [Wiring up your app](06-wiring-apps.md).
