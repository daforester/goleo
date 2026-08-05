# Host features and the mobile provider bridge

> Split out of `AGENTS.md`, which loads into every session. **Read it before adding or changing a runtime/<feature>/ package, its mobile provider, or the generated backend entry points.**
>
> `AGENTS.md` carries the invariants that must be known even without reading this;
> everything here is the detail behind them. `SPIKES.md` has the evidence and the
> hardware-verification history.

---
## Host features via the bridge

(Introduced Jul 2026; the dated narrative is in `docs/history.md`.)

### Bridge Graceful Degradation
- Bridge now handles connection timeout → local-only mode (no backend fallback)
- `backend` config option (a **boolean**, `bridge/src/types.ts`): set false to skip the
  backend entirely and go straight to local-only mode, e.g. for a PWA build
- `showNotification`, `showAlert`, etc. fall back to browser Notification API
- `getOSInfo`, `getPlatformInfo`, `getArch`, `getEnv`, `openURL` fall back to browser APIs when Go backend unavailable

### init.js Restored
- `init.js` is a *feature*, not stale — gives JS developers control over window creation
- Back in `tmplInitJS` in `templates.go`, `new.go` files map, and `create-app.ts` files map
- Embedded via `//go:embed init.js` in main.go template alongside `//go:embed all:frontend/dist`

### Host Features via Bridge
- Architectured a permission-gated host features system (like Tauri/Electron capabilities)
- Each feature is a `runtime/<feature>/` sub-package with platform-specific implementations behind build tags
- Desktop features split from mobile via `runtime/desktop.go` (`//go:build !android && !ios`) calling `RegisterClipboard`, etc.
- `RegisterBuiltins()` reduced to core-only (OS info, env, openURL, notifications); `RegisterDesktopFeatures()` for desktop extras
- Mobile-only features use `goleo_*` build tags (e.g. `goleo_nfc`, `goleo_ble`) so only the
  bindings an app needs are compiled. The Android **manifest** is separate: its permissions
  are derived from the `Register*` calls `detectFeatureUsage` finds, plus
  `mobile.android.extra_permissions` (`cli/cmd/android_permissions.go`) — NOT from the
  compiled tag set, which is deliberately a superset (`nativeShellProviderTags` forces eight
  tags into every build so gobind emits the symbols the fixed Java shell references)
- The same file derives **`<uses-feature>` declarations** (`androidHardwareFeatures`), all
  emitted `android:required="false"`. This is not cosmetic: aapt2 *implies* a `<uses-feature>`
  from certain permissions and an implied entry defaults to `required="true"`, which makes Play
  filter the app off every device lacking that hardware — so **every feature a declared
  permission implies must be listed explicitly**, not just one per goleo feature.
  `TestEveryImpliedHardwareFeatureIsDeclaredOptional` enforces that against a
  permission→implied-feature table. Found on a real Play upload: goleo declared three features
  and Play reported six, with `android.hardware.bluetooth` (from the legacy `BLUETOOTH`
  permissions) and `android.hardware.location` (from `ACCESS_FINE_LOCATION`) silently required.
  Nothing local can catch this — implied features exist only after aapt2 runs, and the symptom
  is a store-side distribution filter.
- `cli/cmd/scan.go` — source scanner that detects `runtime.Register*()` calls and emits the corresponding build tags + manifest entries
- `runtime/clipboard/` — implemented feature with read/write text via platform shell commands; re-exported via `runtime/clipboard_reexport.go`
- `runtime/dialogs/` — native dialogs (file open/save, folder picker, message box, input prompt) via PowerShell (Windows), osascript (macOS), zenity (Linux)
- `runtime/fs/` — file system access (read/write text+binary, list dir, delete, app/home dirs) with path traversal protection
- `runtime/geolocation/` — geolocation via Go backend (stub on desktop, needs `goleo_geolocation` tag on mobile) with full browser API fallback
- `bridge/src/clipboard.ts`, `dialogs.ts`, `fs.ts`, `geolocation.ts` — TS convenience wrappers with browser API fallbacks, all exported from `@goleo/bridge`
- `cli/cmd/generate.go` — `goleo generate types` command that generates `frontend/src/goleo.d.ts` with typed `invoke()` overloads for all 48+ built-in commands

### Complete Host Feature Set (14 features)
All 14 features in `featureRegistry` (`cli/cmd/scan.go`) implemented with Go sub-packages +
re-export bridge handlers + TS convenience wrappers with browser API fallbacks. The table
below lists 13 — **Share** is the 14th (`runtime/share/`, `goleo_share`, native URL hand-off
on all three desktops, provider on mobile, Web Share API fallback). The KV **store**
(`runtime/store/`) is a separate subsystem, not a permission-gated feature, so it is not in
the registry — see Storage below:

Every feature package now exposes a `Provider` interface + `SetProvider`/`runtime.Set<Feature>Provider`, so a mobile shell (or a future native backend) can register a real implementation instead of relying on the `_mobile.go` "no provider registered" error. Desktop status below is the *built-in Go implementation*, not just "compiles":

| Feature | Go Pkg | Build Tag | Desktop | Mobile | TS Browser Fallback |
|---------|--------|-----------|---------|--------|---------------------|
| **Core (9)** | `runtime/` (builtins) | — | Native | Provider | navigator/Notification |
| **Clipboard** | `runtime/clipboard/` | `goleo_clipboard` | Native (Win32 clipboard API / pbcopy / xclip) | Provider | `navigator.clipboard` |
| **Dialogs** | `runtime/dialogs/` | `goleo_dialog` | Native (PowerShell/osascript/zenity) | Provider | `<input type="file">` |
| **FileSystem** | `runtime/fs/` | `goleo_fs` | Native | Provider | Requires Go |
| **Geolocation** | `runtime/geolocation/` | `goleo_geolocation` | Native on Windows (WinRT Geolocator) and macOS (CoreLocationCLI, opt-in); unsupported on Linux | Provider | `navigator.geolocation` |
| **Battery** | `runtime/battery/` | `goleo_battery` | Native (Win32 API / `/sys/class/power_supply` / `pmset`) | Provider | `navigator.getBattery()` |
| **WakeLock** | `runtime/wakelock/` | `goleo_wakelock` | Native (`SetThreadExecutionState` / `caffeinate` / `systemd-inhibit`) | Provider | `navigator.wakeLock` |
| **Vibration** | `runtime/vibration/` | `goleo_vibration` | Unsupported (no desktop vibrator) | Provider | `navigator.vibrate()` |
| **Sensors** | `runtime/sensors/` | `goleo_sensors` | Unsupported (no portable desktop sensor API) | Provider | Generic Sensor API |
| **Camera** | `runtime/camera/` | `goleo_camera` | Unsupported — intentionally routes to WebView `getUserMedia` | Provider | `getUserMedia` + canvas |
| **Bluetooth** | `runtime/bluetooth/` | `goleo_ble` | Unsupported — intentionally routes to Web Bluetooth | Provider | Web Bluetooth API |
| **NFC** | `runtime/nfc/` | `goleo_nfc` | Linux only, opt-in: a cgo libnfc backend behind `-tags goleo_libnfc` (needs libnfc-dev + a reader); unsupported on Windows/macOS | Provider | Web NFC API |
| **Background** | `runtime/background/` | `goleo_background` | Unsupported — desktop process runs continuously, no OS scheduler needed | Provider | Service Worker Sync |
| **Push** | `runtime/push/` | `goleo_push` | Unsupported — use the app's own WebSocket channel instead | Provider | Push API + Service Worker |

"Unsupported" packages return `fmt.Errorf("...: %w", errors.ErrUnsupported)` rather than a generic error, so callers can `errors.Is(err, errors.ErrUnsupported)` to detect "no native path on this platform, use the fallback" instead of a real failure. On Android, the Android WebView (`cli/cmd/templates/{android,android-dev}/.../MainActivity.java`) now wires `WebChromeClient.onPermissionRequest` (camera/mic) and `onGeolocationPermissionsShowPrompt` to runtime permission requests, so the getUserMedia/geolocation browser fallbacks actually work instead of silently failing; on iOS, `AppDelegate.swift` sets a `WKUIDelegate` that grants the equivalent WKWebView permission callbacks, and `Info.plist` declares the required `NS*UsageDescription` strings.

### Fully Generated Backend Entry Points
`backend/main.go` (desktop) and `backend/gomobile/{gomobile.go,notifier.go}` (mobile) are no longer scaffolded once and left as editable source — they're pure boilerplate (call `app.New(...)`, nothing app-specific) regenerated fresh by `generateBackendEntrypoints()` (`cli/cmd/generate_backend.go`) before every `goleo new`/`dev`/`build`/`emulate` run, exactly like the Android/iOS shell templates under `cli/cmd/templates/`. All app-specific logic — commands, feature wiring, `Width`/`Height`/`Port`/`Title` — lives entirely in `backend/app/app.go`, the one file a developer edits. Each generated file carries a `// Code generated by goleo. DO NOT EDIT.` header. A new `.gitignore` (`tmplGitignore` in `templates.go`, mirrored in `create-app.ts`) excludes the three generated files plus `.goleo/`, build outputs, and `node_modules` — none of that previously had a `.gitignore` at all. `backendPkgDir()` (`build.go`) now detects the `backend/` layout by checking for the directory itself rather than `backend/main.go`, since that file may not exist yet on a fresh clone before the first CLI run regenerates it. A new `parseModuleName()` helper (`replace.go`) reads the module path out of `go.mod` at CLI runtime so these files can be rendered outside of `goleo new` (where `projectConfig.ModuleName` was previously only ever constructed once, from the CLI arg).
