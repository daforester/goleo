# __GOLEO_APP_NAME__

A cross-platform app built with [Goleo](https://github.com/daforester/goleo) —
a Go backend paired with a Vue frontend that ships to **desktop**, **Android**,
**iOS**, and the **web (PWA)** from one codebase.

The starter ships with a landing page and a **demo browser** (`frontend/src/demos/`)
that exercises every bridge feature. Use it to see what works on each platform,
then delete the demos you don't need (see [Removing the demos](#removing-the-demos)).

---

## Prerequisites

- **Node.js** 18+ and npm
- **Go** 1.26+
- The **`goleo`** CLI on your `PATH`
- Platform extras, only when you target them:
  - **Desktop Linux:** WebKitGTK (`webkit2gtk-4.1`) + GTK3 dev packages
  - **Android:** handled for you — `goleo emulate android` installs the SDK/NDK,
    a JDK, and an emulator on first run
  - **iOS:** macOS with Xcode

---

## Running

All commands are also available as npm scripts (`npm run goleo:dev`, etc.).

| Task | Command |
| --- | --- |
| Desktop dev (hot reload) | `goleo dev` |
| Web/PWA dev (no Go backend) | `goleo dev pwa` |
| Build desktop for this OS | `goleo build` |
| Cross-compile | `goleo build windows` / `linux` / `darwin` |
| Build a PWA | `goleo build pwa` |
| Run on an Android emulator | `goleo emulate android` |
| Build an Android APK | `goleo build android` |
| Build for iOS (macOS only) | `goleo build ios` |

`goleo dev` starts the Go backend and the Vite dev server together; edits to the
frontend hot-reload, and Go changes restart the backend.

> **Local development note:** until the packages are published, link the bridge
> once in `frontend/`:
> ```bash
> cd frontend && npm link @goleo/bridge && npm install && cd ..
> ```

---

## Project structure

```
__GOLEO_APP_NAME__/
├── goleo.json              App + mobile config (package id, min SDK, …)
├── backend/
│   ├── app/app.go          Wire up features & commands here (all targets)
│   ├── commands/commands.go Your Go functions exposed to the frontend
│   └── init.js             Optional desktop window bootstrap script
└── frontend/
    ├── index.html
    └── src/
        ├── main.ts         Boots Vue + the bridge
        ├── App.vue         Landing page + demo router (make it yours)
        ├── router.ts       Tiny hash router for the demo browser
        ├── style.css       Design tokens & shared styles
        └── demos/          One .vue file per bridge feature
            ├── registry.ts  The list of demos (single source of truth)
            ├── support.ts   Platform detection + support levels
            └── DemoFrame.vue Shared demo chrome (title, badges, back link)
```

Files under `backend/main.go` and `backend/gomobile/` are **generated** on every
`dev`/`build`/`emulate` run — don't edit them.

---

## The bridge, in one minute

The frontend talks to native capabilities through `@goleo/bridge`. Two patterns:

**1. Call your own Go functions** (`invoke`) and **stream events** (`on` / `sendEvent`):

```ts
import { invoke, on, sendEvent } from '@goleo/bridge'

const { message } = await invoke<{ message: string }>('greet', { name: 'Goleo' })
const off = on('heartbeat', (data) => console.log(data)) // returns an unsubscribe fn
sendEvent('app:log', { text: 'hello from the UI' })
```

Register the Go side in `backend/commands/commands.go` with `b.Handle("greet", …)`
and `b.Emit("heartbeat", …)`.

**2. Use built-in features** — typed helpers that call native code with an
automatic browser fallback:

```ts
import { getBatteryInfo, clipboardWriteText, getCurrentPosition } from '@goleo/bridge'

await clipboardWriteText('copied!')
const battery = await getBatteryInfo()
const pos = await getCurrentPosition({ enableHighAccuracy: true })
```

Every demo in `frontend/src/demos/` is a worked example of one feature.

---

## Best practices

- **Always `await initBridge()` before using the bridge.** `main.ts` already
  does this; keep bridge calls inside components/lifecycle hooks, not at module
  top level.
- **Wrap feature calls in `try/catch`.** A feature may be unsupported on the
  current platform; the call throws a descriptive error rather than crashing.
  The demos show the pattern (an `err` ref rendered in a `.result--error` box).
- **Unsubscribe from events.** `on()` returns an unsubscribe function — call it
  in `onBeforeUnmount` to avoid leaks (see `BackendDemo.vue`).
- **Design for "no backend".** In PWA mode there is no Go process; feature
  helpers fall back to browser APIs where possible, and backend-only features
  (filesystem, custom `invoke` handlers) will throw. Gate that UI with
  `isConnected()`.
- **Check permissions first.** For notifications, geolocation, camera, etc.,
  request permission (`requestPermission()`) before the first use.
- **Keep native work in Go.** Do heavy or privileged work in
  `backend/commands` and return results over the bridge, rather than in the UI.
- **Mind secure contexts.** Browser fallbacks for camera, sensors, wake lock,
  Bluetooth and push require HTTPS (or `localhost`).

---

## Platform support & mobile build tags

Native features on **desktop** are wired up in `backend/app/app.go`
(`RegisterDesktopFeatures` plus battery, wake lock and geolocation).

On **mobile**, permission-gated features are compiled in only when you opt in
with build tags, so your app requests only the permissions it needs:

```bash
goleo build android -- -tags "goleo_nfc,goleo_ble,goleo_camera"
```

Then uncomment the matching `runtime.RegisterXxx(a.Bridge())` calls in
`backend/app/app.go`. Available tags include `goleo_nfc`, `goleo_ble`,
`goleo_camera`, `goleo_sensors`, `goleo_vibration`, `goleo_wakelock`,
`goleo_battery`, `goleo_geolocation`, `goleo_background`, `goleo_push`.

Each demo page shows a support badge per platform (Desktop / Android / iOS /
PWA) and highlights the one you're currently running on.

---

## Removing the demos

The demo browser is self-contained and easy to strip out:

- **Remove one demo:** delete its `frontend/src/demos/<Name>Demo.vue` file and
  its single entry in `frontend/src/demos/registry.ts`.
- **Remove all of it:** delete `frontend/src/demos/` and `frontend/src/router.ts`,
  then replace the `<template>` in `frontend/src/App.vue` with your own root
  component. Nothing else references them.

You can also trim `backend/app/app.go` to register only the features you keep.
