# Using `github.com/AndroidGoLab/jni` in a Goleo app

## What it is

[`AndroidGoLab/jni`](https://github.com/AndroidGoLab/jni) is a Go library providing idiomatic Go↔JNI bindings for roughly 137 Android SDK classes across 53 packages (Bluetooth, NFC, location, sensors, telephony, and more), generated from YAML specs. It lets Go code call Android system APIs directly — no Java glue code, no `gomobile bind` interface bridging.

- **Install**: `go get github.com/AndroidGoLab/jni`, then import per-package, e.g. `github.com/AndroidGoLab/jni/bluetooth`.
- **Build requirements**: `CGO_ENABLED=1`, Android NDK, Android SDK, JDK.
- **JNI context**: manages its own JVM/JNI context via `VM`/`Env` types (`VM.Do()` for thread safety) — it attaches to the JVM already running inside the app process via the standard `JNI_OnLoad` mechanism, the same way `gomobile`'s own generated `.so` does. Not a separate JVM.
- **License**: CC0-1.0 (public domain).
- **Maturity**: small and young — 5 stars, 1 fork, 206 commits, 8 tags as of this writing. Treat correctness of the generated bindings as unverified; test carefully before relying on any given class.

Example usage from the repo:

```go
adapter, _ := bluetooth.NewAdapter(ctx)
defer adapter.Close()

enabled, _ := adapter.IsEnabled()
name, _ := adapter.GetName()
```

## Does it work with Goleo's Android build? Yes.

Verified against Goleo's actual `cli/cmd/build.go`/`emulate.go` pipeline:

- `gomobile bind` is invoked as `bind -tags <bindTags> -o goleo.aar -target android -androidapi <n> ./backend/gomobile`. The env it sets (`PATH`, `JAVA_HOME`, `ANDROID_HOME`, `ANDROID_NDK_HOME`) never forces `CGO_ENABLED=0` for this path — that's only done for desktop cross-compiles. Cgo is available by default for the Android/iOS bind step.
- `backend/gomobile` (and anything it imports transitively, e.g. via `backend/app`) is just a normal Go package handed to `gomobile bind` as-is. There's no import inspection or restriction in Goleo's tooling — any valid Go module compiles right along with your app code.
- NDK toolchain/clang/sysroot selection is entirely `gomobile`'s own internal logic, driven by `ANDROID_NDK_HOME`. Goleo only resolves that path and passes `-androidapi`.

So: **a developer can `go get` this library and import it straight into `backend/app/app.go` (or a new file under `backend/gomobile/`), and it should cross-compile through `goleo build android` / `goleo emulate android` with zero framework changes.**

## The one real gap: Android permissions

Goleo regenerates `AndroidManifest.xml` from an embedded CLI template on **every** `goleo build android` / `goleo emulate android` (`cli/cmd/embed.go`'s `extractMobileTemplate()` calls `os.WriteFile` unconditionally, no existence check, no merge). There is currently no `goleo.json` field or other extension point for adding project-specific permissions.

**Practical consequence**: if the Android API you're calling through `AndroidGoLab/jni` needs a permission Goleo's baked-in manifest doesn't already declare, any manual edit to `AndroidManifest.xml` gets silently wiped on your next build.

Permissions Goleo's manifest **already declares** (so these `AndroidGoLab/jni` packages should work today with no changes):

| Permission | Covers |
|---|---|
| `CAMERA`, `RECORD_AUDIO` | camera |
| `ACCESS_FINE_LOCATION`, `ACCESS_COARSE_LOCATION` | location |
| `VIBRATE` | vibration |
| `NFC` | nfc |
| `BLUETOOTH_SCAN`, `BLUETOOTH_CONNECT` (+ legacy `BLUETOOTH`/`BLUETOOTH_ADMIN` for API ≤30) | bluetooth |
| `INTERNET`, `ACCESS_NETWORK_STATE`, `POST_NOTIFICATIONS` | networking/notifications |

Anything needing a permission outside this list (telephony, contacts, calendar, SMS, broad storage access, etc.) is blocked until Goleo's manifest template gains it — which today means patching `cli/cmd/templates/{android,android-dev}/app/src/main/AndroidManifest.xml` and rebuilding the `goleo` CLI itself, not something a project-level change can do.

## The other gap: callback-driven APIs

`AndroidGoLab/jni` is a good fit for **pollable / one-shot** APIs — call a method, get a value back:

- Read battery level, check Bluetooth adapter state, get last known location, query a sensor value on demand.

It's a poor fit for **callback-driven** APIs — anything where Android itself needs to invoke you when something happens:

- NFC tag discovery (needs `Activity.onNewIntent` + foreground dispatch registration)
- Runtime permission results (needs `onRequestPermissionsResult`)
- Continuous sensor streams, lifecycle events, `WebChromeClient` file-picker callbacks

A JNI-calling Go library reached via its own `VM.Do()` can call *into* Java, but nothing calls back *into* it unless something on the Java/Activity side is registered to receive that callback first. This is exactly why Goleo's own NFC/BLE implementation uses native Java provider classes in `MainActivity.java` bound via `gomobile bind`, rather than a pure-Go JNI approach — `BluetoothGatt` read/write and NFC tag discovery are both callback-driven.

## Guidance for developers

1. **Good candidates**: one-shot reads via already-permitted APIs — Bluetooth adapter info, location fixes, sensor snapshots, telephony info (if you add the permission — see below), audio/vibration control.
2. **Before adding a new permission-gated feature**: check the table above. If the permission isn't listed, you'll need to patch Goleo's own `AndroidManifest.xml` templates and rebuild the CLI — file an issue/PR against Goleo rather than trying to work around it per-project, since any manifest edit inside a scaffolded project gets overwritten on the next build.
3. **Callback-driven features**: don't try to force these through `AndroidGoLab/jni` alone. Either wait for Goleo to add native support (following the `gomobile bind` + Java-provider pattern used for Battery/WakeLock/Sensors/Background/NFC/BLE), or add your own Java glue in a fork of the CLI templates.
4. **Test rigorously**: this is a small, young project (5 stars, CC0). Don't take binding correctness on faith — write a small isolated test for each specific class/method before depending on it in a real feature.
5. **Where to put the import**: add it as a dependency of your project's `go.mod` and import it directly in `backend/app/app.go` (or a new file under `backend/app/` or `backend/gomobile/`) — no changes to Goleo's CLI or generated entry points are needed for the Go-code side.

## If Goleo wanted first-class support for this pattern (not implemented, scoped for reference)

The only structural gap is the manifest override. A small addition would close it: a `goleo.json` field like

```json
{
  "android": {
    "extra_permissions": ["android.permission.READ_PHONE_STATE"],
    "extra_features": []
  }
}
```

merged into the manifest as a post-processing step right after `extractMobileTemplate()` runs (in `build.go`/`emulate.go`), before Gradle builds. That one change would make every one-shot `AndroidGoLab/jni` package usable per-project without touching Goleo's own source.
