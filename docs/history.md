# Dated session history

Moved out of `AGENTS.md`, which is loaded into every session: these are **dated records of
work already done**, not guidance. They are kept because they say what was verified, on what
hardware, and what the failure looked like — but nothing here needs to be in context to work
on the code.

For the *conclusions* these led to, see `AGENTS.md`. For the de-risking evidence behind the
architecture, see `SPIKES.md`. For the store-submission state, see `docs/store-submission.md`.

This is a **dated log**: append, and mark superseded entries with a currency note rather than
rewriting them.

---

## Session Summary (Jul 8, 2026) — PWA target and template cleanup

> The rest of this session's summary described the host-features architecture and the
> generated backend entry points, which are current state rather than history; those stayed
> in `AGENTS.md` under "Host features via the bridge".
### PWA Build Target
- Added `goleo build pwa` — builds PWA (no Go backend, frontend only to `dist-pwa/`)
- Added `goleo dev pwa` — starts Vite dev server without Go backend
- Sets `VITE_GOLEO_PLATFORM=pwa` env var for both dev and build


### Template Cleanup
- Removed stale entries from `create-app.ts`: `commands/commands.go`, `commands/init.js`
- `init.js` is at `backend/init.js`; commands are at `backend/commands/commands.go`
  (the tree at the top of this file is the accurate one)


---

## Session Summary (Jul 16, 2026) — icons + Android validated end-to-end

- **App-icon generation (all platforms), pure Go.** New `cli/cmd/icons.go` (+ `icons_test.go`)
  turns a single `bundle.icon` PNG into every platform artifact; wired into `winres.go` (Windows
  exe, now multi-size), `bundle.go` (macOS `.icns`, Linux hicolor PNG + generated `.desktop`), and
  the mobile build path. See Distribution → Icons above and `docs/guide/04-packaging-icons.md`.
- **Mobile launcher icons.** Android `mipmap-*/ic_launcher(+_round).png` (all densities, round via
  a circular alpha mask) referenced from a `{{if .HasIcon}}`-gated `android:icon` in the manifest;
  iOS `AppIcon.appiconset` gated by `ASSETCATALOG_COMPILER_APPICON_NAME` in `xcodegen.yml`.
- **Fixed: mobile builds broke for vendored projects and for any app not enabling every feature.**
  Two real bugs, both in `SPIKES.md`:
  1. A scaffolded project commits `vendor/` → Go picks `-mod=vendor`, but `gomobile bind` needs
     `golang.org/x/mobile` bind-support packages absent from `vendor/`, and `go get -tool` refuses
     to run under vendor mode. Mobile build path now forces `GOFLAGS=-mod=mod` (`modModEnv`/
     `goToolEnv`/`setMobileEnv` in `cli/cmd/gotools.go`,`build.go`).
  2. The native shell (`MainActivity.java`/`AppDelegate.swift`) unconditionally wires all 8 native
     providers, but their gomobile bindings were gated behind per-feature `goleo_*` tags only
     enabled when the app called `Register*` — so the default scaffold failed with `cannot find
     symbol gomobile.BatteryProvider`. `mobileBindTags` (`scan.go`) now always binds
     `nativeShellProviderTags` (battery/wakelock/sensors/background/nfc/ble/clipboard/share);
     per-feature *bridge-command registration* stays opt-in via `Register*`.
- **Verified on a real Android emulator (API 36, x86_64):** `goleo build android` on a freshly
  scaffolded+vendored project produced a 66 MB APK (4 ABIs, all launcher-icon densities, all demo
  permissions), installed and launched; the Go backend + bridge ran (invoke + push events), and
  **native providers round-tripped live** — battery (real level), clipboard (write→read), FS
  (write→read via `SetHomeDir`), wake-lock, sensors (confirmed registered with the OS
  SensorManager), share. Camera/BLE/NFC need real peripherals (not on the emulator) but their
  bindings compile + are wired and permissions are present. NSIS bundle verified end-to-end on
  Windows (real installer, no path doubling) via `nsis_integration_test.go`.

## Session Summary (Aug 4, 2026) — iOS tier 1 (Simulator, no Apple account)

iOS was the only target with no verification of any kind. "Tier 1" is everything reachable
**without a paid Apple Developer account and without Mac hardware**, i.e. on a GitHub
`macos-14` runner: build for the Simulator, install, launch, prove it runs.

- **`goleo build ios --simulator`** — compiles against the Simulator SDK with
  `CODE_SIGNING_ALLOWED=NO`, so it needs no certificate. gomobile now binds
  `-target ios,iossimulator` so the `.xcframework` carries a simulator slice at all (the
  load-bearing assumption; it is the first step of the CI job for that reason). `--simulator`
  is refused on non-iOS targets.
- **Four iOS template defects fixed**, each of which only a real build or a real submission
  would surface (asserted in `cli/cmd/ios_template_test.go` so they cannot regress on a host
  without Xcode):
  1. `xcodegen.yml` took `PRODUCT_BUNDLE_IDENTIFIER` from the **Android** `package_name`, so
     `mobile.ios.bundle_identifier` was dead config and both apps shared one identity.
  2. `Info.plist` hardcoded `CFBundleName` "Goleo App", version `1.0`, build `1` — every app
     shipped under the framework's name and no version bump reached the bundle.
  3. `Info.plist` set `UILaunchStoryboardName` to a `LaunchScreen` that **did not exist in the
     template** — a black launch screen and an App Store rejection.
  4. No `PRODUCT_NAME`, so the product was named after the target (`App.app`) while the CLI
     printed `GoleoApp.app`, a path it never wrote. `buildForIOS` now verifies the bundle
     exists and lists what it found instead of printing a fixed success line.
- **One source of truth for each platform's minimum OS version** (`cli/cmd/mobile_minversion.go`).
  gomobile builds the Go library against a minimum (`-iosversion`/`-androidapi`) and the native
  project declares its own (`deploymentTarget`, `minSdk`); these had independent sources — a CLI
  flag versus `goleo.json` — and on iOS they **disagreed with no configuration at all**
  (`--ios-target` defaulted to 14.0, `mobile.ios.deployment_target` to 15.0). A library minimum
  *above* the app's fails to link, naming a version the user never chose. `--ios-target` /
  `--android-api` now default to empty and override the config rather than competing with it.
- **CI: an `ios-simulator` job** in `mobile-verify.yml` — probe the simulator slice, scaffold,
  build, verify the bundle (name, executable, LaunchScreen, `CFBundleName` is not the
  placeholder), boot a simulator with `xcrun simctl`, install, launch, assert still running,
  upload a screenshot.

**Status after the real macOS runs (2026-08-04/05): iOS builds, installs and runs.** Each run
peeled off one layer that had never been exercised, in order:
1. the simulator-slice probe **passed** — gomobile does emit `ios-arm64-simulator`;
2. `xcodebuild` refused the project: XcodeGen's default `projectFormat` writes objectVersion
   77 and the runner's Xcode is 15.4 — pinned to `xcode14_0`;
3. `error: There is no XCFramework found at .goleo/ios/App.xcframework` — the template asked
   for `App.xcframework` while the build wrote `goleo.xcframework`; both are now
   `Goleo.xcframework`, which is also what gomobile derives the Swift module name from
   (`bind_iosapp.go`: `title = strings.Title(base minus ".xcframework")`, `Module: title`);
4. `App/AppDelegate.swift` had never been compiled, and the two names it needs come from
   different places — the Swift module is `Goleo` (from the artifact name) while gobind
   derives the Objective-C class prefix from the Go *package* (`gomobile` → `Gomobile`). So
   `import Goleo` was right but `Goleo.setHomeDir(...)` and `GoleoNotifierProtocol` were not.
   Fixed against the generated headers, which the job prints;
5. `actool` refused the build: XcodeGen's own `settingPresets` set
   `ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon` for an application target, so goleo's
   `HasIcon` gate could add that setting but not prevent it, and the scaffold ships no icon.
   Overridden with an explicit empty value;
6. a launch crash found by reading rather than running: the BGTask identifier came from the
   **Android** package name while `Info.plist` permits `$(PRODUCT_BUNDLE_IDENTIFIER).sync`.
   Registering an unpermitted identifier raises an NSException from `didFinishLaunching`.
   Harmless only while the iOS build reused the Android id — making
   `mobile.ios.bundle_identifier` take effect in 0.10.2 is what turned it into a crash.

**Resolved (2026-08-05): iOS builds, installs and runs.** A `macos-14` runner produced a
`GoleoApp.app` from a fresh scaffold, installed it on a simulator and launched it, and the
screenshot shows the embedded UI rendered with `goleo:getOS` round-tripped from the Go
backend (`"os":"ios","arch":"arm64"`), a custom invoke returning, and `heartbeat` push
events arriving — so the backend, loopback asset serving, WKWebView, bridge invoke and
event push all work. The remaining iOS gap is distribution (`.ipa`/TestFlight), which needs
a paid Apple Developer account.

**Tier 2 (not done, and not blocked on hardware):** signed `.ipa` via `archive` +
`-exportArchive`, `ExportOptions.plist`, entitlements, TestFlight upload. All of that needs a
paid Apple Developer account, which a GitHub runner cannot substitute for. Mac App Store stays
gated behind the acceptance spike (`docs/roadmap.md`) — the App Sandbox forbids the
self-replacing updater outright.

