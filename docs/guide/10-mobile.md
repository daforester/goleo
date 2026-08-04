# 10. Mobile

Goleo runs the same Go backend + web UI inside a native Android/iOS shell (the
platform WebView hosts the UI; the Go backend runs in-process via gomobile). This
page covers the device workflows.

> Prereqs: Android SDK **platform-tools** (`adb`) + **NDK** for Android; Xcode for
> iOS (macOS only). See [Installation](01-installation.md).

## Develop on a real device (Android)

```bash
npm run goleo:dev-android        # = goleo emulate android
```

This will:
1. Use a **connected device** if one is attached (USB debugging on); otherwise it
   starts an emulator.
2. Build the dev APK (gomobile AAR + Android project) and install it.
3. `adb reverse` the frontend port so the WebView loads over
   `http://localhost:<vitePort>` — a **secure context**, so camera, clipboard, and
   geolocation work in dev (loading over `10.0.2.2` would silently disable them).
4. Run the Go backend inside the app on port 9842; the UI hot-reloads from Vite.

Attach a device first:
```bash
adb devices          # confirm your phone shows up (authorize the prompt on-device)
```

## Sideload a build (Android)

Build an installable APK and push it to the connected device:

```bash
npm run goleo:sideload-android   # builds app.apk, then adb install + launch
```

Under the hood: `goleo build android` (produces an unsigned debug `app.apk`) then `goleo install
android` (finds the connected device, `adb install -r`, launches the activity).
Run the install step alone if you already have an APK:

```bash
goleo install android            # installs ./app.apk onto the connected device
goleo install android --apk out.apk --launch=false
```

`goleo install` requires a connected device / running emulator and errors clearly
if none is present (it will not spin up an emulator).

## iOS

> **iOS does not build yet — as of 0.10.2 it stops at the Swift compile.** Android is
> the supported mobile target. Everything below describes the intended workflow and the
> commands are real, but `App/AppDelegate.swift` — the native shell that hosts the
> WebView and wires the platform providers — was written without ever being compiled,
> and its gomobile binding names are wrong. It is being fixed against the real generated
> headers on a macOS runner rather than guessed at; follow
> [the roadmap](../roadmap.md) for status.
>
> What *does* work as of 0.10.2: gomobile emits a device **and** simulator slice, the
> generated Xcode project opens in Xcode 14 and newer, the project and the build agree on
> the framework name, and `Info.plist` carries your app's real name, version and bundle
> identifier. Those were all broken, and each one had to be fixed before the Swift
> failure was even reachable.

```bash
npm run goleo:build-ios          # -> GoleoApp.app, a DEBUG build (macOS + Xcode)
goleo build ios --simulator      # -> GoleoApp.app for the Simulator, unsigned
```

`goleo build ios` is intended to produce a finished `GoleoApp.app`; the `.xcframework`
gomobile generates is an intermediate that the build consumes and deletes, so there is
nothing for you to integrate by hand.

**Without an Apple Developer account**, use `--simulator`. It builds against the
Simulator SDK with code signing disabled, which is the only iOS path that needs no
certificate at all — and once the shell compiles, this is how you run it:

```bash
goleo build ios --simulator
xcrun simctl list devices available          # pick one
xcrun simctl boot "iPhone 16"
open -a Simulator
xcrun simctl install booted GoleoApp.app
xcrun simctl launch booted com.example.myapp # your mobile.ios.bundle_identifier
```

`xcrun simctl spawn booted log stream --predicate 'processImagePath CONTAINS "GoleoApp"'`
tails the app's logs, which is the closest equivalent to `adb logcat`.

**On a device**, open the generated project in Xcode and run it from there — a device
build needs a signing certificate and a provisioning profile. There is no `.ipa`
export yet, so TestFlight and App Store submission are not wired up.

(`goleo emulate`/`goleo install` are Android-only.)

## Permissions

Goleo auto-grants the app's own permission prompts so the frontend's browser-API
fallbacks resolve instead of hanging:

- **Android**: the WebView wires `onPermissionRequest` (camera/mic) and
  `onGeolocationPermissionsShowPrompt` to runtime permission requests; declare the
  matching Android manifest permissions for what you use.
- **iOS**: the `WKUIDelegate` grants the WebView callbacks; `Info.plist` must
  declare the `NS*UsageDescription` strings (camera, mic, location…).

## Host features on mobile

Desktop-native features (clipboard, dialogs, fs, geolocation, battery, …) route to
**platform providers** on mobile. Where a feature has no native path on a given
platform it returns `errors.ErrUnsupported`, and the `@goleo/bridge` wrapper falls
back to the Web API when one exists (e.g. `getUserMedia`, Web Bluetooth, Web NFC).
Mobile-only capabilities (NFC, BLE, sensors, push, background sync…) are compiled
in only when you register them, keeping the Android manifest minimal.

## Identity & icons

Set the package name / bundle id and launcher icon per
[Packaging](04-packaging-icons.md#mobile-identity):

```jsonc
"mobile": {
  "android": {
    "package_name": "com.example.myapp",
    "min_sdk": 24,
    "target_sdk": 36,
    // Play rejects an upload whose versionCode has not increased. Omit it and one is
    // derived from the top-level "version" (1.2.3 -> 10203); GOLEO_ANDROID_VERSION_CODE
    // overrides both, so CI can stamp a build number without editing this file.
    "version_code": 10203,
    // Permissions are derived from the features your app enables. Anything detection
    // cannot see — a capability used only through a frontend browser API — goes here.
    "extra_permissions": ["RECORD_AUDIO"]
  },
  "ios": { "deployment_target": "15.0", "bundle_identifier": "com.example.myapp" }
}
```

`goleo build android` prints the permissions it derived and the feature that asked for
each, so a missing one is visible while you build rather than after a user installs — on
Android an undeclared permission fails silently, with the runtime request returning
"denied" and no prompt.

## Tips

- Serve dev over `localhost` (Goleo does this via `adb reverse`) — never
  `http://10.0.2.2`, which isn't a secure context and disables camera/clipboard/geo.
- Cross-compiling mobile from any host works for Android (NDK); iOS requires macOS.
- Keep backend work off the UI thread; use events to push results to the WebView.

---

Back to the [Guide index](README.md).
