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

Under the hood: `goleo build android` (produces `app.apk`) then `goleo install
android` (finds the connected device, `adb install -r`, launches the activity).
Run the install step alone if you already have an APK:

```bash
goleo install android            # installs ./app.apk onto the connected device
goleo install android --apk out.apk --launch=false
```

`goleo install` requires a connected device / running emulator and errors clearly
if none is present (it will not spin up an emulator).

## iOS

```bash
npm run goleo:build-ios          # -> .xcframework (macOS + Xcode)
```

Integrate the `.xcframework` into an Xcode project and run/deploy through Xcode,
the Simulator, TestFlight, or the App Store. (`goleo emulate`/`install` are
Android-only today.)

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
  "android": { "min_sdk": 24, "package_name": "com.example.myapp" },
  "ios":     { "deployment_target": "14.0", "bundle_identifier": "com.example.myapp" }
}
```

## Tips

- Serve dev over `localhost` (Goleo does this via `adb reverse`) — never
  `http://10.0.2.2`, which isn't a secure context and disables camera/clipboard/geo.
- Cross-compiling mobile from any host works for Android (NDK); iOS requires macOS.
- Keep backend work off the UI thread; use events to push results to the WebView.

---

Back to the [Guide index](README.md).
