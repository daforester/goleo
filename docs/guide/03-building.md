# 3. Building

Goleo produces three kinds of output:

- **Standalone binaries** — a single self-contained executable (frontend embedded).
- **Native installers** — see [Packaging, icons & metadata](04-packaging-icons.md).
- **Mobile packages** — an Android `.apk`/`.aab`, or an iOS `.app`.

## Standalone binaries

```bash
npm run goleo:build            # current OS/arch
npm run goleo:build-windows    # Windows amd64  -> app.exe
npm run goleo:build-linux      # Linux amd64
npm run goleo:build-darwin     # macOS amd64
```

Or directly:
```bash
goleo build            # current platform
goleo build windows    # or linux / darwin
goleo build -o myapp   # custom output name
```

What happens: the frontend is built with Vite, embedded into the Go binary via
`//go:embed`, and a single executable is produced that serves its own UI. No
runtime files, no external server to deploy.

### Cross-compilation

Every desktop target builds `CGO_ENABLED=0`, so you can build **all** desktop
platforms from one machine — no per-OS toolchain:

```bash
goleo build windows   # from macOS or Linux
goleo build darwin    # from Windows or Linux
goleo build linux     # from Windows or macOS
```

(Per-OS machines are still needed to *sign/notarize* and to build *installers*
whose packager only runs on that OS — but not to compile.)

The named targets pin `amd64`; `--arch` overrides it for any desktop target,
including `current`:

```bash
goleo build linux --arch arm64      # arm64 Linux from an amd64 machine
goleo build --arch arm64            # arm64 build of the host OS
```

`--arch` does not apply to `android`, `ios` or `pwa` and is refused there —
Android ABIs are selected with `--android-abi` instead:

```bash
goleo build android --android-abi arm64-v8a     # one ABI: ~4x smaller than the default
```

By default all four ABIs are built (arm64-v8a, armeabi-v7a, x86_64, x86), which is
what makes a default APK ~66 MB. GOARCH names are accepted too. Note an emulator
needs `x86_64`, so a single-ABI `arm64-v8a` build will not install on one.

`--no-sign` skips code signing on every platform even when credentials are
configured — useful in CI for a build you only want to check compiles.

`--android-api` and `--ios-target` set the minimum OS version the **Go library** is
compiled against. Leave them alone unless you have a reason: they default to
`mobile.android.min_sdk` / `mobile.ios.deployment_target` from `goleo.json`, which is also
what the native project declares. The two must agree — a Go library with a *higher*
minimum than the app fails to link — so overriding one flag without changing the config
is how you break that.

## PWA

```bash
npm run goleo:build-pwa        # -> dist-pwa/  (static site, no Go backend)
```

The bridge degrades gracefully: `invoke()` calls that have a browser equivalent
(clipboard, notifications, geolocation, file pickers…) fall back to the Web API;
calls that require Go return an error you can handle.

## Mobile

```bash
npm run goleo:build-android    # -> app.apk (installable)
npm run goleo:build-ios        # -> GoleoApp.app, a DEBUG build (macOS + Xcode)
```

- **Android**: builds a gomobile AAR, generates an Android project, and compiles
  an unsigned debug `app.apk` with Gradle. Needs the Android SDK + NDK.

  For something you can actually ship, add `--release`:

  ```bash
  goleo generate android-key                    # creates release.jks, prints the env vars
  goleo build android --release                 # signed app.aab for Play
  goleo build android --release --android-format apk   # signed APK for outside a store
  ```

  `--release` **errors** without a keystore rather than quietly producing an unsigned
  artifact, since neither Play nor a device will accept one. `--no-sign` is the explicit
  way to build one anyway. `goleo generate android-key` uses the JDK goleo already
  resolves for Gradle, so it works when `keytool` is not on your PATH — which it usually
  is not, as it lives inside the JDK's `bin/`.

  The generated manifest declares only the permissions your app enables, and the build
  prints the list with the feature that asked for each one. If something is missing, add
  it to `mobile.android.extra_permissions` in `goleo.json`.
- **iOS**: builds a **debug** `GoleoApp.app` with `xcodebuild`. macOS only. The `.xcframework`
  gomobile produces is an intermediate that the build consumes and then deletes, so there
  is nothing to integrate by hand.

  ```bash
  goleo build ios                 # device build; needs a signing certificate
  goleo build ios --simulator     # Simulator build; needs NO Apple account
  ```

  `--simulator` is the path that needs no Apple Developer account: it compiles against the
  iOS Simulator SDK with code signing off, so you can run the app in the Simulator on any
  Mac with Xcode. Install and launch it with:

  ```bash
  xcrun simctl boot "iPhone 16"          # or any device in `xcrun simctl list devices`
  xcrun simctl install booted GoleoApp.app
  xcrun simctl launch booted <your bundle id>
  ```

  The bundle identifier comes from `mobile.ios.bundle_identifier`, the minimum OS version
  from `mobile.ios.deployment_target`, and the version/build numbers from the project
  `version` — see [Setup](02-setup.md). `--ios-target` overrides the deployment target for
  one build.

  There is still **no `.ipa`** — see [the roadmap](../roadmap.md) — so TestFlight and App
  Store distribution are not wired up. Both need a paid Apple Developer account.

To run on a real device during development, or to sideload the APK, see
[Mobile](10-mobile.md).

## The webview backend

The desktop backend is the cgo-free **glaze** binding on all three OSes (WKWebView /
WebKitGTK / WebView2 via `purego`) — there is nothing to configure or install. The
former cgo `webview_go` and Windows `go-webview2` fallbacks have both been removed,
so every desktop build is `CGO_ENABLED=0` and cross-compiles from any host with no C
toolchain.

## What the version string is

Builds stamp the binary with `goleo.json`'s `version` (via `-ldflags -X
main.Version=...`), and — on Windows — embed it in the executable's version info
(see the next page).

---

Next: [Packaging, icons & metadata →](04-packaging-icons.md)
