# 6. Wiring up your app

Everything app-specific lives in **`backend/app/app.go`** — it builds a
`runtime.App` from a `Config`, registers your commands, and runs. The generated
`main.go`/mobile entry points just call into it.

## The shape of `app.go`

```go
package app

import (
    "context"
    "embed"

    goleo "github.com/daforester/goleo/runtime"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

func New() *goleo.App {
    app := goleo.New(goleo.Config{
        Title:      "My App",
        Width:      1024,
        Height:     768,
        WindowMode: goleo.WindowModeWebview,   // native desktop window
        EmbedFS:    frontendFS,
        NativeIPC:  true,                       // in-process bridge (recommended desktop)
        OnStartup:  func(ctx context.Context) { /* before the window opens */ },
        OnReady:    func(ctx context.Context) { /* server + window manager up; OpenWindow works */ },
        OnShutdown: func(ctx context.Context) { /* clean up */ },
    })

    goleo.RegisterBuiltins(app.Bridge())          // core commands (OS info, notify, openURL…)
    goleo.RegisterDesktopFeatures(app.Bridge())   // clipboard, dialogs, fs, … (desktop)

    registerCommands(app)                          // your commands (below)
    return app
}
```

## Config essentials

| Field | Purpose |
|-------|---------|
| `Title`, `Width`, `Height` | Window title + size |
| `WindowMode` | `WindowModeWebview` (native window) / `WindowModeBrowser` (PWA/dev) / `WindowModeMobile` |
| `EmbedFS` | Your embedded `frontend/dist` |
| `DevMode`, `DevServer` | Dev: load from the Vite server instead of embedded files |
| `NativeIPC` | Route the bridge over the webview's in-process channel (no socket) |
| `SchemeAssets`, `AssetScheme` | Serve the UI from a portless secure origin (`goleo://`) — see [RPC](07-rpc.md) |
| `OnStartup` / `OnReady` / `OnShutdown` | Lifecycle hooks (`OnReady` is where `OpenWindow` works) |
| `Menu` | Native menu bar — see [Menus](08-menus.md) |
| `Tray`, `Background` | System tray / headless controller — see [System tray](09-systray.md) |
| `SingleInstance`, `AppID` | Allow only one instance; forward args to the running one |
| `URLScheme` | Register a `myapp://` deep-link scheme |
| `InProcessWindows` | Open extra windows in-process (vs child processes) |

## Registering commands (backend → callable from JS)

A command is a Go function the frontend calls by name:

```go
func registerCommands(app *goleo.App) {
    app.Bridge().Handle("add", func(ctx context.Context, args json.RawMessage) (any, error) {
        var p struct{ A, B int }
        if err := json.Unmarshal(args, &p); err != nil {
            return nil, err
        }
        return map[string]int{"sum": p.A + p.B}, nil
    })
}
```

The return value is JSON-encoded back to the caller; a non-nil `error` becomes a
rejected promise on the frontend. Full details in [RPC in depth](07-rpc.md).

## Events (push, both directions)

```go
// Backend → frontend (push):
app.Emit("data:updated", map[string]any{"count": 42})

// Frontend → backend (fire-and-forget):
app.On("app:ready", func(ctx context.Context, data json.RawMessage) {
    log.Println("frontend is ready")
})
```

## The frontend side (`@goleo/bridge`)

```ts
import { initBridge, invoke, on } from '@goleo/bridge'

await initBridge()                                  // auto-detects the transport

const { sum } = await invoke('add', { a: 2, b: 3 }) // call a Go command
const off = on('data:updated', (d) => render(d))    // subscribe to a backend event
// ...later: off()
```

`initBridge()` picks the best transport automatically — **native IPC** in a
desktop webview, **WebSocket** otherwise, **HTTP** as a last resort — all with the
same API. Generate typed `invoke()` overloads with `npm run goleo:types`.

## Host features (native capabilities)

Beyond your own commands, Goleo ships permission-gated host features, each callable
via `invoke('goleo:...')` **or** a typed convenience wrapper from `@goleo/bridge`,
with a browser fallback where one exists:

| Area | Examples | Bridge wrapper |
|------|----------|----------------|
| Core | OS/platform info, `openURL`, notifications | `getOSInfo()`, `openURL()`, … |
| Clipboard | read/write text | `clipboard.*` |
| Dialogs | open/save file, folder picker, message/prompt | `dialogs.*` |
| File system | read/write text+binary, list, app-data dir | `fs.*` |
| Geolocation, battery, wake-lock, … | platform-gated | typed wrappers |
| Store | small key/value persistence | `store.*` (localStorage fallback) |

Register the ones you use on the backend (e.g. `RegisterDesktopFeatures`, or
individual `runtime.Register*` calls); unused features aren't compiled in. On
mobile, features route to platform providers; on PWA, to the Web API where
available.

**→ See the full working tour of every host feature — notifications, dialogs,
clipboard, filesystem, geolocation, battery, camera, Bluetooth, NFC, share, store,
and more — in [Host features](11-host-features.md).**

## Multiple windows

```go
app.OpenWindow(goleo.WindowOptions{ Path: "/settings", Width: 500, Height: 400 })
```
`OpenWindow` / `CloseWindow` / `ListWindows` also have bridge commands
(`goleo:window{Open,Close,List}`) and `@goleo/bridge` wrappers
(`openWindow`/`closeWindow`/`listWindows`), plus `window:opened`/`window:closed`
events. Call `OpenWindow` from `OnReady` or at runtime, not `OnStartup`.

## Quitting

```go
app.Quit()   // idempotent: unblocks Run → closes windows → OnShutdown → stops server
```
Also exposed as `goleo:quit` / `@goleo/bridge`'s `quitApp()`.

## Scripting in JavaScript (`backend/init.js`)

`backend/init.js` runs inside the Go backend's embedded JS engine before any window opens.
It sets up windows, and it is a place to put logic you want to change without recompiling —
pricing rules, formatting, per-customer tweaks.

**Go calls into it:**

```js
// backend/init.js
function priceOrder(o) { return o.qty * o.unit * 1.2 }
```

```go
// backend/app/app.go
total, err := a.JS().Call(ctx, "priceOrder", order)          // -> any
err = a.JS().CallJSON(ctx, "priceOrder", &out, order)        // decode into a struct
if a.JS().Has(ctx, "priceOrder") { /* optional hook */ }
```

**And it calls back out:**

```js
goleo.invoke("goleo:notify", { title: "Done" })   // any bridge command
goleo.emit("job:finished", { id: 7 })             // push an event to the frontend
```

Four things worth knowing before you lean on it:

- **Every call takes a context, and you should give it a deadline.** A script with an
  accidental infinite loop would otherwise wedge the engine for the life of the process. With
  a deadline it is interrupted and the runtime survives for the next call.
- **`goleo.invoke` is synchronous and goes through the same `Policy` ACL as the frontend.**
  If a policy is set and does not allow the command, the script gets a
  `permission denied` throw — catchable with `try`/`catch`.
- **A JS exception becomes a Go error**, so a broken script fails the call rather than the app.
- **Arguments and results cross as JSON**, so pass plain data. A Go struct works; a channel,
  function or `chan`-bearing type does not.

Run `goleo generate types` to refresh `backend/init.d.ts`, which gives your editor
autocomplete for `getConfig`, `createWindow` and `goleo`, and flags anything else as
undefined.

Delete `init.js` (and its `//go:embed` line in the generated `main.go`) to fall back to plain
`runtime.Config` window setup.

---

Next: [RPC in depth →](07-rpc.md)
