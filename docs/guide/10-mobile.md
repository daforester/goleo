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

### Microphone capture on the emulator

`RegisterMicrophone` declares **two** Android permissions, and the second is the one that
catches people out:

- `RECORD_AUDIO` — the runtime prompt you see and approve.
- `MODIFY_AUDIO_SETTINGS` — a **normal** permission, granted silently at install.

Chromium's WebView media stack requires **both** before it will enumerate any input device.
Without the second it logs `Requires MODIFY_AUDIO_SETTINGS and RECORD_AUDIO. No audio device
will be available for recording` and `getUserMedia({audio:true})` throws `NotReadableError` —
with nothing on screen to suggest a permission is involved, because the prompt you *did* see
was granted. goleo declares both; this is only worth knowing if you hand-roll a manifest.

On an emulator, two further things are needed and goleo handles the first:

- **`-allow-host-audio` at launch.** Without it the emulator "zeroes out audio" (its own
  `-help` text). goleo passes it whenever it starts the emulator; an emulator you started
  yourself has to be restarted through `goleo emulate android` to get it.
- **`hw.audioInput=yes` in the AVD's `config.ini`** — the virtual mic hardware. Most device
  profiles set it already. `goleo doctor android` reports both of these without starting
  anything.

If capture still fails after that, check **Extended Controls (⋮) → Microphone → "Virtual
microphone uses host audio input"**, your Windows **default recording device** (the emulator
opens whatever Windows considers default), and **Settings → Privacy & security → Microphone →
"Let desktop apps access your microphone"**.

A real device sidesteps the emulator-specific items entirely, which is why the device
checklists use one.

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

```bash
npm run goleo:build-ios          # -> GoleoApp.app, a DEBUG build (macOS + Xcode)
goleo build ios --simulator      # -> GoleoApp.app for the Simulator, unsigned
```

`goleo build ios` produces a finished `GoleoApp.app`; the `.xcframework`
gomobile generates is an intermediate that the build consumes and deletes, so there is
nothing for you to integrate by hand.

A device build is signed, so it needs an Apple Developer Team ID in
`mobile.ios.development_team` (or `--ios-team`). Setting it in Xcode does not work —
goleo regenerates the project under `.goleo/ios/` on every build.

**Without an Apple Developer account**, use `--simulator`. It builds against the
Simulator SDK with code signing disabled, which is the only iOS path that needs no
certificate at all:

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
  "ios": { "deployment_target": "15.4", "bundle_identifier": "com.example.myapp" }
}
```

`goleo build android` prints the permissions it derived and the feature that asked for
each, so a missing one is visible while you build rather than after a user installs — on
Android an undeclared permission fails silently, with the runtime request returning
"denied" and no prompt.

It also prints the **hardware features** those permissions imply:

```
Hardware features (all optional): android.hardware.bluetooth, android.hardware.bluetooth_le,
android.hardware.camera, android.hardware.location, android.hardware.location.gps,
android.hardware.nfc
```

Every one is declared `android:required="false"`, and that matters more than it looks.
Android derives a `<uses-feature>` from certain permissions on its own, and an **implied**
entry is *required* by default — which makes Play filter your app off every device without
that hardware. Since goleo's features fall back to a browser API rather than being
essential, each is declared optional explicitly. Play will still list them; check
*Release → Bundle explorer → Features* against the printed list, and watch your supported
device count if you ever add a hardware permission by hand.

## Tips

- Serve dev over `localhost` (Goleo does this via `adb reverse`) — never
  `http://10.0.2.2`, which isn't a secure context and disables camera/clipboard/geo.
- Cross-compiling mobile from any host works for Android (NDK); iOS requires macOS.
- Keep backend work off the UI thread; use events to push results to the WebView.

---

Back to the [Guide index](README.md).
