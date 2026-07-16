# 11. Host features — the demo tour

Goleo ships a set of **host features** — native capabilities your web UI can call:
notifications, clipboard, file dialogs, filesystem, geolocation, battery, camera,
Bluetooth, NFC, and more. This page is a working tour of all of them.

> Want a ready-made, runnable version of everything below? Scaffold the demo:
> `goleo new my-app --demo` — a project with a live page per feature.

## How host features work

1. **Register** the features you use on the Go side (unused ones aren't compiled
   in — smaller binaries, minimal Android manifest).
2. **Call** them from the frontend via a typed `@goleo/bridge` wrapper, or via
   `invoke('goleo:...')` directly (both hit the same handler).
3. **Fallback**: where a platform lacks a native path, the call routes to the Web
   API when one exists (e.g. `getUserMedia`, Web Bluetooth, Web NFC), or returns
   `errors.ErrUnsupported` so you can branch.
4. **Detect** support at runtime with the capabilities query before showing UI.

### Registration (Go — `backend/app/app.go`)

```go
b := app.Bridge()
goleo.RegisterBuiltins(b)          // core: OS info, env, openURL, notifications, capabilities
goleo.RegisterDesktopFeatures(b)   // clipboard + dialogs + filesystem (desktop)

// Opt into the rest à la carte:
goleo.RegisterGeolocation(b)
goleo.RegisterBattery(b)
goleo.RegisterWakeLock(b)
goleo.RegisterVibration(b)
goleo.RegisterSensors(b)
goleo.RegisterCamera(b)
goleo.RegisterBLE(b)               // Bluetooth Low Energy
goleo.RegisterNFC(b)
goleo.RegisterShare(b)
goleo.RegisterStore(b)             // key/value persistence
goleo.RegisterPush(b)
goleo.RegisterBackground(b)        // background sync
```

### Capability detection (frontend)

```ts
import { getCapabilities, isTraySupported } from '@goleo/bridge'
const caps = await getCapabilities()          // { windowing, tray, menu, ... }
if (caps.menu) { /* show a "customize menu" button */ }
```
Or per-feature: features return `ErrUnsupported` / a wrapper rejects, so a
`try/catch` around the call is always safe.

### A note on the demo snippets

Each section shows the `@goleo/bridge` wrapper **and** the raw `invoke('goleo:...')`
form (they're equivalent). Import what you use:

```ts
import { invoke, sendNotification, openFile, readText, getBatteryInfo } from '@goleo/bridge'
```

---

## Core: OS info, env, open URL

Registered by `RegisterBuiltins`. Always available (desktop, mobile, PWA).

```ts
import { getOSInfo, getPlatformInfo, getArch, getEnv, openURL } from '@goleo/bridge'

const os = await getOSInfo()          // { name, arch, version }   — or invoke('goleo:getOS')
const plat = await getPlatformInfo()  // { type: 'desktop' | 'mobile' | 'pwa' }
const home = await getEnv('HOME')     // whitelisted env var        — invoke('goleo:getEnv', { key })
await openURL('https://example.com')  // open in the default browser — invoke('goleo:openURL', { url })
```

## Notifications

`RegisterBuiltins`. Native on desktop/mobile; Web Notification API on PWA.

```ts
import { sendNotification, isPermissionGranted, requestPermission } from '@goleo/bridge'

if (!(await isPermissionGranted())) await requestPermission()
await sendNotification({ title: 'Build finished', body: 'All 42 tests passed' })
// raw: invoke('goleo:notify', { title, body })
```

## Clipboard

`RegisterClipboard` (in `RegisterDesktopFeatures`). Native on desktop; `navigator.clipboard` fallback.

```ts
import { readText, writeText } from '@goleo/bridge'   // clipboard

await writeText('copied from Go!')      // invoke('goleo:clipboardWriteText', { text })
const text = await readText()           // invoke('goleo:clipboardReadText') -> { text }
```

## File dialogs

`RegisterDialogs` (in `RegisterDesktopFeatures`). Native pickers on desktop;
`<input type="file">` fallback in the browser.

```ts
import { openFile, openFiles, saveFile, selectFolder, showMessage, showPrompt } from '@goleo/bridge'

const path  = await openFile({ filters: [{ name: 'Text', extensions: ['txt', 'md'] }] })
const many  = await openFiles({})                       // string[]
const out   = await saveFile({ defaultPath: 'notes.txt' })
const dir   = await selectFolder({})
const btn   = await showMessage({ title: 'Delete?', message: 'This cannot be undone', buttons: ['Cancel', 'Delete'] })
const name  = await showPrompt({ title: 'Rename', message: 'New name:', defaultValue: 'untitled' })
// raw: invoke('goleo:dialogOpenFile' | 'goleo:dialogSaveFile' | 'goleo:dialogSelectFolder' | 'goleo:dialogShowMessage' | 'goleo:dialogShowPrompt', { ... })
```

## File system

`RegisterFS` (in `RegisterDesktopFeatures`). Native on desktop (with path-traversal
protection); requires the Go backend (no PWA equivalent).

```ts
import { readTextFile, writeTextFile, readBinaryFile, writeBinaryFile,
         listDir, deleteFile, appDataDir, homeDir } from '@goleo/bridge'

const dir = await appDataDir()                          // per-app data directory
await writeTextFile(`${dir}/notes.txt`, 'hello')        // invoke('goleo:fsWriteTextFile', { path, content })
const s   = await readTextFile(`${dir}/notes.txt`)      // invoke('goleo:fsReadTextFile', { path })
const entries = await listDir(dir)                      // FileEntry[]  (name, isDir, size, ...)
await deleteFile(`${dir}/notes.txt`)
```

## Geolocation

`RegisterGeolocation`. Native on Windows (WinRT) + macOS (opt-in); Linux
unsupported → `navigator.geolocation` fallback; native on mobile.

```ts
import { getCurrentPosition } from '@goleo/bridge'
const pos = await getCurrentPosition({ enableHighAccuracy: true })  // { coords: { latitude, longitude, ... } }
// raw: invoke('goleo:geolocationGetCurrentPosition', { enableHighAccuracy })
```

## Battery

`RegisterBattery`. Native (Win32 / `/sys/class/power_supply` / `pmset`);
`navigator.getBattery()` fallback.

```ts
import { getBatteryInfo } from '@goleo/bridge'
const b = await getBatteryInfo()   // { level, charging }   — invoke('goleo:batteryGetInfo')
```

## Wake lock

`RegisterWakeLock`. Native (`SetThreadExecutionState` / `caffeinate` /
`systemd-inhibit`); `navigator.wakeLock` fallback.

```ts
import { wakeLockRequest, wakeLockRelease } from '@goleo/bridge'
await wakeLockRequest()   // keep the screen/system awake
// ...long task...
await wakeLockRelease()
```

## Vibration

`RegisterVibration`. Mobile only (no desktop vibrator); `navigator.vibrate()` in PWA.

```ts
import { vibrate } from '@goleo/bridge'
await vibrate(200)                 // ms  — invoke('goleo:vibrate', { pattern: 200 })
```

## Sensors

`RegisterSensors`. Mobile via provider; the Generic Sensor API in the browser.
Sensor readings stream back as `goleo:sensorReading` events.

```ts
import { startSensor, stopSensor, on } from '@goleo/bridge'
const off = on('goleo:sensorReading', (r) => console.log(r))   // { type, x, y, z, ... }
await startSensor('accelerometer')     // invoke('goleo:sensorStart', { type })
// ...later:
await stopSensor('accelerometer'); off()
```

## Camera

`RegisterCamera`. Desktop intentionally routes to the WebView's `getUserMedia`;
mobile via provider. `capturePhoto` returns image data.

```ts
import { capturePhoto } from '@goleo/bridge'
const photo = await capturePhoto()     // { data, mimeType }  — invoke('goleo:cameraCapturePhoto')
// For a live stream, use getUserMedia in the WebView (a secure context is required —
// enabled by SchemeAssets / the loopback origin).
```

## Bluetooth (BLE)

`RegisterBLE`. Desktop/mobile route to Web Bluetooth (secure context required);
provider-backed on mobile where available.

```ts
import { requestDevice, connect, disconnect } from '@goleo/bridge'
const device = await requestDevice({ filters: [{ services: ['battery_service'] }] })  // goleo:bleRequestDevice
await connect(device.id)                 // goleo:bleConnect
// read/write characteristics: invoke('goleo:bleRead' | 'goleo:bleWrite', { ... })
await disconnect(device.id)              // goleo:bleDisconnect
```

## NFC

`RegisterNFC`. Web NFC fallback; provider on mobile. Tag reads arrive as events.

```ts
import { startScan, stopScan, on } from '@goleo/bridge'
const off = on('goleo:nfcTag', (tag) => console.log(tag))
await startScan()                        // goleo:nfcStartScan
// write: invoke('goleo:nfcWrite', { records: [...] })
await stopScan(); off()                  // goleo:nfcStopScan
```

## Share

`RegisterShare`. Native share sheet on mobile; Web Share API fallback.

```ts
import { share } from '@goleo/bridge'
await share({ title: 'Check this out', text: 'Cool app', url: 'https://example.com' })  // goleo:share
```

## Key/value store

`RegisterStore`. A JSON file in the app-data dir (atomic writes); `localStorage`
fallback in the browser.

```ts
import { storeGet, storeSet, storeDelete, storeKeys, storeClear } from '@goleo/bridge'
await storeSet('theme', 'dark')          // goleo:storeSet  { key, value }
const theme = await storeGet('theme')    // goleo:storeGet  -> value
const keys  = await storeKeys()          // string[]
await storeDelete('theme'); await storeClear()
```

## Push notifications

`RegisterPush`. Provider-backed on mobile; Push API + Service Worker in PWA.

```ts
import { subscribe, unsubscribe, getSubscription } from '@goleo/bridge'
const sub = await subscribe({ /* vapidPublicKey, etc. */ })  // goleo:pushSubscribe
const cur = await getSubscription()                          // goleo:pushGetSubscription
await unsubscribe()                                          // goleo:pushUnsubscribe
```

## Background sync

`RegisterBackground`. Provider on mobile; Service Worker Background Sync in PWA
(desktop runs continuously, so it's a no-op there). Sync runs emit a
`goleo:backgroundSync` event.

```ts
import { registerSync, on } from '@goleo/bridge'
await registerSync('outbox')             // goleo:backgroundRegisterSync { tag }
on('goleo:backgroundSync', ({ tag }) => flush(tag))
```

---

## Platform support at a glance

| Feature | Desktop | Mobile | PWA fallback |
|---------|---------|--------|--------------|
| Core / notifications | Native | Native | navigator / Notification |
| Clipboard | Native | Provider | `navigator.clipboard` |
| Dialogs | Native | Provider | `<input type=file>` |
| File system | Native | Provider | — (needs Go) |
| Geolocation | Win/macOS native | Native | `navigator.geolocation` |
| Battery | Native | Provider | `navigator.getBattery()` |
| Wake lock | Native | Provider | `navigator.wakeLock` |
| Vibration | — | Provider | `navigator.vibrate()` |
| Sensors | — | Provider | Generic Sensor API |
| Camera | getUserMedia | Provider | `getUserMedia` |
| Bluetooth | Web Bluetooth | Provider | Web Bluetooth |
| NFC | — | Provider | Web NFC |
| Share | — | Native | Web Share |
| Store | Native | Native | `localStorage` |
| Push | — | Provider | Push API + SW |
| Background | — | Provider | SW Background Sync |

"—" means no native desktop path; the wrapper still falls back to the Web API in a
WebView/PWA where one exists.

## Secure context reminder

Camera, Bluetooth, geolocation, clipboard, and service workers require a **secure
context**. Goleo provides one automatically — the loopback origin, or the portless
`goleo://` origin when `SchemeAssets` is enabled (see [RPC](07-rpc.md)). In mobile
dev, Goleo serves over `localhost` via `adb reverse` for the same reason.

## Related native integrations

These live on their own pages/commands but are part of the same host surface:
- **[Native menus](08-menus.md)** — `setMenu` / `onMenu`.
- **[System tray](09-systray.md)** — `Config.Tray`.
- **Windows** — `openWindow` / `closeWindow` / `listWindows`.
- **Auto-update** — `checkForUpdate` / `applyUpdate` (see [Deploy](05-deploy.md)).
- **Autostart** — `enableAutostart` / `disableAutostart` / `isAutostartEnabled`.
- **Deep links** — `getInitialURL` / `onDeepLink` (register `Config.URLScheme`).

---

Back to the [Guide index](README.md).
