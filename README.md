# Goleo

**Build cross-platform desktop and mobile apps with a Go backend and a web UI.**

Write your app logic in **Go**. Build your interface with **any web framework** — Vue, React,
Svelte, or plain HTML. Ship one codebase to **Windows, macOS, Linux, Android, iOS, and the
web (PWA)**.

Goleo is in the Tauri / Wails class — a compiled backend driving the OS's native webview — but
the backend is Go, and a single project targets desktop *and* mobile *and* PWA.

```go
app.Bridge().Handle("greet", func(ctx context.Context, args json.RawMessage) (any, error) {
    var p struct{ Name string `json:"name"` }
    _ = json.Unmarshal(args, &p)
    return "Hello, " + p.Name + "!", nil
})
```
```ts
import { invoke } from '@goleo/bridge'
const msg = await invoke<string>('greet', { name: 'World' })   // → "Hello, World!"
```

---

## 📖 Developer Guide

Full step-by-step docs live in **[`docs/guide/`](docs/guide/README.md)** —
installation, project setup, building, packaging (icons + metadata + installers),
deploying & auto-update, wiring up your app, RPC, native menus, system tray, and
mobile (device dev + sideloading).

## Why Goleo

- 🖥️ **One codebase, six targets** — Windows, macOS, Linux, Android, iOS, PWA.
- 🐹 **Real Go on the backend** — full concurrency, the standard library, and the Go ecosystem
  for your app logic and system access.
- 🎨 **Any web frontend** — Vue (default template), React, Svelte, vanilla — anything Vite builds.
- 🔌 **Typed bridge** — call Go from JS with `invoke()`, push events from Go with `Emit()`;
  generate TypeScript types for every command.
- 📦 **Small single binary** — the frontend is embedded via `//go:embed`; no bundled Chromium
  (uses the OS webview).
- 🚀 **Ships itself** — installers, code signing/notarization, and a **signed auto-updater**
  built into the CLI.
- 🪟 **Native desktop integration** — multi-window, system tray, single-instance, launch-on-login,
  deep links, and a signal-based lifecycle.
- 🔒 **Secure by design** — an opt-in runtime capability ACL and a hardened loopback bridge.
- 📱 **Device features** — clipboard, filesystem, dialogs, share, camera, geolocation, battery,
  sensors, notifications, and more — native where it counts, graceful browser fallbacks elsewhere.

---

## Quick start

```bash
# Install the CLI (npm, or `go install github.com/daforester/goleo/cli/goleo@latest`)
npm install -g @goleo/cli

# Scaffold a project (Vue + Vite + @goleo/bridge)
goleo new my-app                       # or, no global install: npx @goleo/cli new my-app
cd my-app
cd frontend && npm install && cd ..

# Live development (Go backend + Vite HMR)
goleo dev

# Build a single binary for the current platform
goleo build
```

Then edit **`backend/app/app.go`** (startup + feature wiring), **`backend/commands/`** (your Go
commands), and **`frontend/src/`** (your UI). That's it.

---

## Project layout

```
my-app/
├── goleo.json              # project + bundle/publish config
├── backend/
│   ├── app/app.go          # the one file you edit: config, command + feature registration
│   ├── commands/           # your backend commands
│   ├── main.go             # generated (desktop entry point) — do not edit
│   └── gomobile/           # generated (mobile entry points) — do not edit
└── frontend/               # your web app (Vite; Vue by default, swappable)
    └── src/main.ts         # inits the bridge
```

`main.go` and the `gomobile/` glue are **regenerated on every build** — all your logic lives in
`backend/app/app.go` and `backend/commands/`.

---

## The bridge — calling Go from your UI

**Go — register a command:**
```go
app.Bridge().Handle("saveNote", func(ctx context.Context, args json.RawMessage) (any, error) {
    var note struct{ Title, Body string }
    if err := json.Unmarshal(args, &note); err != nil {
        return nil, err
    }
    // ...persist it...
    return map[string]string{"id": "note-1"}, nil
})

// Push an event to the frontend at any time:
app.Emit("sync:done", map[string]any{"count": 42})
```

**TypeScript — invoke it and listen for events:**
```ts
import { invoke, on } from '@goleo/bridge'

const { id } = await invoke<{ id: string }>('saveNote', { title: 'Hi', body: '...' })
const unsubscribe = on('sync:done', (data) => console.log('synced', data))
```

Run `goleo generate types` to produce `frontend/src/goleo.d.ts` with a fully typed `invoke()`
for every built-in command.

---

## Desktop capabilities

Configure in `runtime.Config` and call the bridge helpers from `@goleo/bridge`.

**Multi-window, tray, and lifecycle:**
```go
a = runtime.New(runtime.Config{
    Title:            "My App",
    InProcessWindows: true,               // Windows: extra windows in-process (else child processes)
    SingleInstance:   true,               // a second launch focuses the running one
    URLScheme:        "myapp",            // register myapp:// deep links
    Background:       true,               // headless controller (window(s) on demand)
    Tray: &runtime.TrayConfig{            // optional system tray
        Tooltip: "My App",
        Items: []runtime.TrayItem{
            {Label: "Open", OnClick: func() { a.OpenWindow(runtime.WindowOptions{Path: "/"}) }},
            {Label: "Quit", OnClick: func() { a.Quit() }},
        },
    },
})
```
```ts
import { openWindow, quitApp, getInitialURL, onDeepLink,
         enableAutostart, checkForUpdate, applyUpdate } from '@goleo/bridge'

await openWindow({ path: '/settings', width: 600, height: 400 })
onDeepLink((url) => route(url))               // myapp:// links while running
await enableAutostart()                       // launch on login
if ((await checkForUpdate()).available) await applyUpdate()
```

| Capability | Config / API |
|---|---|
| Additional windows | `App.OpenWindow` · `openWindow`/`closeWindow`/`listWindows` · `WindowOptions.ExitOnClose` |
| System tray | `Config.Tray` + `Config.Background` |
| Single instance | `Config.SingleInstance` → `app:secondInstance` event |
| Launch on login | `enableAutostart` / `disableAutostart` / `isAutostartEnabled` |
| Deep links | `Config.URLScheme` → `getInitialURL` + `onDeepLink` |
| Graceful quit | `App.Quit()` · `quitApp()` |
| Auto-update | `runtime.RegisterUpdater` + `checkForUpdate`/`applyUpdate` |

---

## Device features & storage

Register the ones you use in `backend/app/app.go`; the CLI auto-detects them for mobile builds.
Each has a TS wrapper with a browser fallback, so PWA/dev degrade gracefully.

```go
runtime.RegisterDesktopFeatures(a.Bridge())  // clipboard, dialogs, filesystem
runtime.RegisterStore(a.Bridge())            // persistent key/value store
runtime.RegisterShare(a.Bridge())            // native share sheet
runtime.RegisterClipboard(a.Bridge())
runtime.RegisterCamera(a.Bridge())           // + geolocation, battery, sensors, vibration, nfc, ble…
```
```ts
import { storeSet, storeGet, share, readText } from '@goleo/bridge'
await storeSet('theme', 'dark')
const theme = await storeGet<string>('theme')
await share({ title: 'Goleo', url: 'https://example.com' })
```

**Available:** clipboard · dialogs · filesystem · **key/value store** · **share** · camera ·
geolocation · battery · sensors · vibration · wake-lock · NFC · Bluetooth (BLE) · notifications ·
background · push.

---

## Security — capability ACL

Bridge access is permissive by default; set a **`Policy`** to switch to deny-by-default and scope
sensitive plugins (Tauri-style, enforced centrally on every `invoke`):

```go
app.SetPolicy(&runtime.Policy{
    Allow:   []string{"goleo:store*", "greet"},   // exact or "prefix*"; core info commands always allowed
    FSRoots: []string{"/home/me/app-data"},        // scope filesystem access
})
```
The loopback bridge is also hardened in production (loopback-only bind, origin allow-list,
per-launch token).

---

## Distribution & auto-update

The CLI takes you from build to a self-updating installer:

```bash
goleo build --bundle             # native installer: .msi/NSIS · .dmg · .deb/.rpm
goleo build --bundle --publish   # + write an ed25519-signed update manifest
goleo generate updater-key       # keypair for signing updates
```
Code signing/notarization are env-driven (`GOLEO_WIN_CERT`, `GOLEO_MAC_IDENTITY`,
`GOLEO_APPLE_ID`, …) so secrets stay out of the repo and CI can inject them. The in-app updater
verifies the signed manifest before applying an update.

---

## CLI reference

| Command | What it does |
|---|---|
| `goleo new <name>` | Scaffold a new project |
| `goleo dev` | Dev mode — Go backend + Vite HMR |
| `goleo dev pwa` | Frontend-only PWA dev (no Go backend) |
| `goleo build [target]` | Build for `current`/`windows`/`linux`/`darwin`/`android`/`ios`/`pwa` |
| `goleo build --bundle` | Also produce a native installer |
| `goleo build --publish` | Also write the signed update manifest |
| `goleo emulate android` | Build + run on a connected Android emulator/device |
| `goleo generate types` | Generate `goleo.d.ts` typed bridge bindings |
| `goleo generate updater-key` | Generate an ed25519 update-signing keypair |

---

## Platform support

| | Desktop app | Mobile app | PWA | Native webview |
|---|:---:|:---:|:---:|---|
| **Windows** | ✅ | — | ✅ | WebView2 (**cgo-free**) |
| **macOS** | ✅ | — | ✅ | WKWebView (cgo) |
| **Linux** | ✅ | — | ✅ | WebKitGTK (cgo) |
| **Android** | — | ✅ | ✅ | system WebView (gomobile) |
| **iOS** | — | ✅ | ✅ | WKWebView (gomobile) |

Windows desktop builds are fully **cgo-free** and cross-compile. macOS/Linux desktop builds use
the system webview via cgo today (a cgo-free in-process backend is on the roadmap). Mobile builds
use `gomobile`; iOS requires macOS + Xcode.

---

## How it works

```
┌────────────────────────┐   invoke / events    ┌────────────────────────┐
│  Web UI (OS webview)    │ ◄──────────────────► │  Go backend + Bridge   │
│  @goleo/bridge          │  WebSocket · HTTP    │  your commands + feats │
└────────────────────────┘  · native bind       └────────────────────────┘
```

- **Dev:** Vite serves the UI with HMR and proxies to the Go backend.
- **Production:** the frontend is embedded in the Go binary (`//go:embed`) and served from a
  hardened loopback server; the OS webview loads it.
- **Mobile:** a `gomobile` `.aar`/`.xcframework` runs the same Go backend inside the platform's
  native WebView.

Deeper docs: [`AGENTS.md`](AGENTS.md) (architecture), [`docs/roadmap.md`](docs/roadmap.md)
(masterplan + status), [`SPIKES.md`](SPIKES.md) (feasibility findings),
[`docs/comparison.md`](docs/comparison.md) (vs Tauri v2 & Wails v3).

---

## Status

Feature-complete and shipping-ready on all targets via the paths above. The one active
refinement is a **cgo-free in-process webview for macOS/Linux** (Windows already has it); see the
roadmap.

## License

MIT
