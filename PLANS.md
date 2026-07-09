# PLANS.md — Extending Goleo's native feature set (Capacitor-style)

> **Purpose of this file:** working plan + cold-start orientation. If you're a fresh
> context, read "Orientation" first — it saves you re-exploring the repo.

---

## Orientation (read this first if you lost context)

**What Goleo is:** a cross-platform app framework in the **Wails/Tauri class** — a **Go
backend** + a **web frontend rendered in the OS WebView**, talking over a loopback
WebSocket/HTTP bridge. Ships to Windows, Linux, macOS, Android, iOS, and PWA from one
codebase. Module: `github.com/daforester/goleo`, Go 1.26.

- **UI** = HTML/CSS/JS (Vue default, any web framework) in a system WebView
  (WebView2 / WebKitGTK / WKWebView on desktop via `github.com/webview/webview_go`;
  Android `WebView` / iOS `WKWebView` on mobile). **No custom renderer, no GPU/canvas
  layer, no native widgets.**
- **Logic** = Go. Frontend calls Go via `@goleo/bridge`: `invoke('method', args)` →
  `Bridge.Handle` on the Go side; events via `on()` ↔ `Emit`.
- **Mobile** = `gomobile` builds a `.aar`/`.xcframework`; a native shell
  (`MainActivity.java` / `AppDelegate.swift`) owns the WebView, starts the embedded Go
  server, and **injects native "provider" implementations into Go** at startup.
- **Embedded JS**: `runtime/jsruntime.go` runs `goja` (pure-Go JS VM) only for the
  optional `init.js` window-setup script. App logic is Go, not Node.
- **Authoritative docs:** `AGENTS.md` (deep architecture), `README.md`,
  `docs/extending-android-with-jni.md`.
- **Watch out:** `cli/npm/goleo/` is a **full mirror copy** of the repo (npm dist), so
  most files appear twice in searches. `runtime/webview/` (dir) is empty; real code is
  `runtime/webview.go`.

**Existing host features (13 + core):** clipboard, dialogs, fs, geolocation, battery,
wakelock, vibration, sensors, camera, bluetooth(BLE), nfc, background, push, plus core
(OS info, env, openURL, notifications). Each is a permission-gated `runtime/<feature>/`
sub-package behind a `goleo_*` build tag, with a TS wrapper that browser-falls-back.

---

## Why this plan exists (the original question)

**"Does React Native give us a shortcut to implementing a lot of features?" → No.**

| | Goleo | React Native |
|---|---|---|
| UI | HTML/JS in the OS **WebView** | **Native widgets** via Fabric + Yoga |
| Logic | **Go** over a loopback bridge | JavaScript on Hermes/JSC |
| Native features | `gomobile` + Java/Swift **providers** injected into Go | TurboModules bound to the RN runtime |

RN's renderer can't slot into a WebView model, and RN native modules are bound to
`ReactContext`/the RN module registry — they can't run in Goleo's `gomobile` shell.
**The matching ecosystem is Capacitor/Cordova** (web UI in a native WebView shell +
plugin bridge = Goleo's exact shape). Use Capacitor plugins' Java/Swift as *porting
references*, not drop-ins.

**Chosen direction:** fill device-feature gaps by extending the host-feature system,
porting from Capacitor plugins. (Two alternatives were declined: adopting a mobile UI
kit like Ionic/Quasar; and "just wanted the analysis".)

---

## The repeatable pattern: one feature = one vertical slice

Canonical reference feature = **`battery`** (has both desktop-native and mobile-provider
paths). To add feature `Foo`, touch every item below.

### Go runtime (`runtime/`)
1. `runtime/foo/foo.go` — `FooInfo` struct, `Provider` interface, `SetProvider`, dispatch
   func (prefer provider, else `platformFoo()`). Tag `//go:build !(android || ios) || goleo_foo`. Mirror `runtime/battery/battery.go`.
2. `runtime/foo/foo_{windows,linux,darwin}.go` — desktop native impl. No portable desktop
   path → return `fmt.Errorf("...: %w", errors.ErrUnsupported)` (TS then uses browser fallback).
3. `runtime/foo/foo_mobile.go` — tag `(android || ios) && goleo_foo`; "no native provider registered".
4. `runtime/foo/foo_stub.go` — disabled-tag stub.
5. `runtime/foo_reexport.go` — `RegisterFoo(b)` doing `b.Handle("goleo:fooXxx", …)`;
   `type FooProvider = foo.Provider` aliases; `SetFooProvider`. Mirror `runtime/battery_reexport.go`.
6. `runtime/desktop.go` — add `RegisterFoo(b)` **only if** on-by-default on desktop
   (like Clipboard/Dialogs/FS). Otherwise opt-in from the app template.

### Frontend bridge (`bridge/src/`)
7. `bridge/src/foo.ts` — `invoke('goleo:fooXxx')` in `try/catch` with browser fallback. Mirror `bridge/src/battery.ts`.
8. `bridge/src/index.ts` — export new fns + types.

### CLI / build wiring (`cli/cmd/`)
9. `cli/cmd/scan.go` — add `Feature{}` to `featureRegistry` (Name, `goleo_foo`,
   `Permissions`, `IOSUsageDescs`); add `RegisterFoo\(` to `scanPatterns`; extend the
   `StringRef`/`ImportRef`/`EventRef` regex alternations with `foo`.
10. `cli/cmd/templates.go` — add `tmplMobileFooGo`: gomobile `FooProvider` interface
    (flat primitive methods only) + `fooAdapter` → `runtime.FooInfo`. Tag
    `mobilebuild && goleo_foo`. Mirror `tmplMobileBatteryGo` (~templates.go:886). Register
    the file in the generated-file map (see `generate_backend.go`).
11. `cli/cmd/generate.go` — add typed `invoke()` overloads for `goleo:fooXxx` to `goleo.d.ts`.

### Native shells (mobile providers)
12. `cli/cmd/templates/android/…/MainActivity.java` **and** `…/android-dev/…/MainActivity.java`
    — `import gomobile.FooProvider;`, `Gomobile.setFooProvider(new GoleoFoo());` in
    `onCreate` (~L134), null it in cleanup, and a `private class GoleoFoo implements FooProvider`.
    Mirror `GoleoBattery` (~MainActivity.java:386). **Keep both copies in sync.**
13. `cli/cmd/templates/ios/App/AppDelegate.swift` — `Goleo.setFooProvider(...)` +
    `GoleoFoo: NSObject, GoleoFooProviderProtocol`. Mirror `GoleoBatteryStatus` (~:102).
    *(iOS path is best-effort/untested per existing comments.)*

### App template + demo
14. `create-goleo-app/template/backend/app/app.go` — commented-out `// runtime.RegisterFoo(a.Bridge())`.
15. `create-goleo-app/template/frontend/src/demos/FooDemo.vue` + entry in `.../demos/registry.ts`.

### THREE HARD GOTCHAS (do not forget)
- **Manifest permissions are NOT auto-injected.** `featureRegistry.Permissions` /
  `IOSUsageDescs` in `scan.go` are declared but **never read** (verified: nothing reads
  them). `cli/cmd/templates/{android,android-dev}/…/AndroidManifest.xml` is a **static**
  template. A feature needing a *new* permission must be hand-added to **both** manifests
  **and** iOS `Info.plist`. Already-declared perms (INTERNET, CAMERA, RECORD_AUDIO,
  ACCESS_FINE/COARSE_LOCATION, VIBRATE, NFC, BLUETOOTH*, POST_NOTIFICATIONS, ACCESS_NETWORK_STATE)
  need no manifest change.
- **Template duplication.** Scaffold templates live in `cli/cmd/templates.go` **and**
  `create-goleo-app/src/create-app.ts`; `cli/npm/goleo/…` is a full mirror. Mirror every
  edit and rebuild dists. (Memory: "Goleo template sync".)
- **gomobile marshaling.** `gobind` only bridges primitives/strings. Provider interfaces
  must be flat methods (see `GoleoBattery.level()/charging()`). Structs/maps cross as
  **JSON strings** (see BLE `requestDeviceJSON`). Callback-driven features also need an
  `emit*` func on the Go side + a listener registration in the shell (mirror Sensors/NFC).

---

## Recommended features (prioritized)

| Feature | Tag | Shape | Desktop native | New Android perm? | Capacitor ref |
|---|---|---|---|---|---|
| **Share sheet** | `goleo_share` | one-shot | partial (Win share / macOS `NSSharingService` / Linux `xdg-open`) | no | `@capacitor/share` |
| **Secure storage** | `goleo_securestore` | one-shot | native (wincred / Keychain / libsecret) | no | `@capacitor/preferences`, `capacitor-secure-storage` |
| **In-app browser** | `goleo_inappbrowser` | one-shot | reuse existing `openURL` | no | `@capacitor/browser` (Custom Tabs / SFSafariVC) |
| **Biometric auth** | `goleo_biometric` | callback | native (Windows Hello / Touch ID) | no (`USE_BIOMETRIC`) | `capacitor-native-biometric` |
| **Contacts** | `goleo_contacts` | one-shot | none | **yes — `READ_CONTACTS`** | `@capacitor-community/contacts` |

**Build order:** do **Share sheet first** as the exemplar — lowest complexity, exercises
the *entire* slice, needs no new permission. Then the other three one-shots. Do
**Contacts last** — it forces the manifest/`Info.plist` edit and motivates the enabler below.

**Optional enabler (do with Contacts):** wire the already-declared
`featureRegistry.Permissions` / `IOSUsageDescs` into manifest + `Info.plist` generation
(post-process after `extractMobileTemplate()` in `build.go`/`emulate.go`). Closes the
static-manifest gap the JNI doc calls out; future permission-gated features then become a
pure `scan.go` registry edit.

---

## Verification

1. **Desktop e2e:** register in `backend/app/app.go`, `goleo dev`, open the demo, exercise
   it, confirm the *native* path fires (not just fallback). Unsupported-desktop → confirms
   clean `errors.ErrUnsupported` fallback.
2. **Detection:** `goleo build`/`goleo scan` prints "Detected mobile features: …" with `goleo_foo`.
3. **Android:** `goleo emulate android`, grant the prompt, confirm the Java provider returns
   real data; verify regenerated `AndroidManifest.xml` has the needed permission.
4. **Types:** `goleo generate types` → `frontend/src/goleo.d.ts` has overloads for `goleo:fooXxx`.
5. **PWA:** `goleo dev pwa` → TS browser fallback or clean "requires backend" error, no Go process.
6. **Rebuild CLI** after template edits (`go build ./cli/...`) and re-sync dists.

---

## Status / next action

- [ ] Not started. First concrete step: implement **Share sheet** end-to-end as the exemplar
  slice (all 15 touch points above), verify on desktop + Android, then replicate for the rest.

## Key files to open (reference copies to mirror)
- Go: `runtime/battery/battery.go`, `runtime/battery/battery_mobile.go`,
  `runtime/battery_reexport.go`, `runtime/desktop.go`
- Bridge: `bridge/src/battery.ts`, `bridge/src/index.ts`
- CLI: `cli/cmd/scan.go`, `cli/cmd/templates.go` (`tmplMobileBatteryGo` ~L886),
  `cli/cmd/generate.go`, `cli/cmd/generate_backend.go`
- Shells: `cli/cmd/templates/android/app/src/main/java/com/goleo/app/MainActivity.java`
  (GoleoBattery ~L386), `cli/cmd/templates/ios/App/AppDelegate.swift`,
  `cli/cmd/templates/{android,android-dev}/app/src/main/AndroidManifest.xml`
- Template/demo: `create-goleo-app/template/backend/app/app.go`,
  `create-goleo-app/template/frontend/src/demos/registry.ts`
- Orientation: `AGENTS.md`, `docs/extending-android-with-jni.md`