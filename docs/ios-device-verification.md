# iOS device verification — run sheet

For whoever has the Mac and the iPhone. None of this has been tested on hardware yet, so
treat every step as "please confirm". The sheet is self-contained — you do not need a goleo
checkout.

Reference environment (what the last run used): iPhone 17 Pro Max on iOS 26.6, Xcode 26.6,
macOS 26.5.2, Go 1.26.5 darwin/arm64, XcodeGen 2.46.0.

---

## 1. Set up

Upgrade the CLI:

```bash
npm install -g @goleo/cli@0.10.10
# or, if you installed with Go:
#   go install github.com/daforester/goleo/cli/goleo@v0.10.10

goleo version      # must print 0.10.10
```

If the CLI reports a *version mismatch* between `@goleo/cli` and its native binary package,
follow its instructions and reinstall — npm can leave the platform binary on an older
release, which silently runs the old CLI.

**Scaffold a fresh demo app — do not reuse an older one.**

```bash
goleo new ios-check --demo
cd ios-check
```

Upgrading the CLI upgrades *most* of an existing app: the native shells under `.goleo/`, the
generated `backend/gomobile/*` files, and the pinned Go runtime are all rewritten on every
build. But `backend/app/app.go` and `frontend/src/**` are yours — scaffolded once and never
regenerated. So an older demo app has **no Microphone page and no `RegisterMicrophone` call**,
and items 13, 13a and 13b below cannot be run in it at all. Everything else would work; that
one section would silently have nothing to click.

Add your Team ID to the app's `goleo.json`:

```jsonc
"mobile": {
  "ios": {
    "bundle_identifier": "...",        // leave whatever is already there
    "development_team": "ABCDE12345"   // <- NEW, 10 characters
  }
}
```

Find it in **Xcode > Settings > Accounts** (select the team; it is shown next to the name),
or under Membership details at developer.apple.com/account. A free Apple ID works — it gives
a personal team that can install on your own device, with the profile expiring after 7 days.

**Do not set the team in Xcode instead.** goleo regenerates the Xcode project under
`.goleo/ios/` on every build and will overwrite it. It must be in `goleo.json`.

> If you *also* want to exercise the upgrade path real users take, run your existing app
> through `goleo build ios` afterwards as a second pass — it should still build and pass
> items 1–12 and 14–19. That is a bonus, not the main run.

---

## 2. Build

Capture both logs.

```bash
goleo build ios --simulator 2>&1 | tee build-sim.log      # expect BUILD SUCCEEDED
goleo build ios            2>&1 | tee build-device.log
```

Then confirm the CLI and the Go runtime agree:

```bash
grep goleo go.mod          # must show github.com/daforester/goleo v0.10.10
```

**If it does not say `0.10.10`, stop and re-run the build.** The Go module tag can lag a
few minutes behind the npm release; goleo says so when it happens (`not tagged as a Go
module yet — using @latest`). A mismatch here shows up as `undefined:
runtime.FileDialogOptions` and means you are testing new code against an old runtime.

Check `build-device.log`, whether or not it succeeded:

- It should print `Compiling for iOS devices with xcodebuild...`
- It must **not** contain `Using the first of multiple matching destinations` followed by
  `platform:macOS`. That means it built a Mac app instead of an iOS one.
- It should not mention `requires a development team`. If it does, Xcode rejected the Team
  ID — usually that team is not on an Apple ID signed into Xcode, or the device is not
  registered to it.

Install `GoleoApp.app` on the phone and launch it.

---

## 3. Checklist

| # | Check | Expected |
|---|---|---|
| 1 | Notification permission prompt appears | Prompt shown |
| 2 | A notification is actually **delivered and visible** | Banner appears while the app is open |
| 3 | Accelerometer readings update | Values change |
| 4 | Gyroscope readings update | Values change |
| 5 | Magnetometer readings update | Values change |
| 6 | Battery level and charging state | Match the device |
| 7 | Clipboard: copy from the app | Pasteable elsewhere |
| 8 | Clipboard: paste into the app | Reads the system clipboard |
| 9 | Share sheet **opens** | System share sheet appears |
| 10 | Wake lock | Screen stays on |
| 11 | Background sync registers | No error |
| 12 | Camera permission prompt appears | Prompt shown |
| 13 | Microphone permission prompt appears | Prompt shown — use the **Microphone** demo's "Request permission" |
| 14 | Location permission prompt appears | Prompt shown |

Microphone page (new — nothing in the demo could reach the mic before, which is why this
item had no result last time):

| # | Check | Expected |
|---|---|---|
| 13a | "Request permission" | The iOS mic prompt appears; status then reads granted |
| 13b | Record, stop, then play the clip back | You hear what you recorded |

Dialogs page:

| # | Check | Expected |
|---|---|---|
| 15 | `showMessage` | Native alert; the button you tap is what comes back |
| 16 | `showPrompt` | Alert with a text field; confirming returns the text, **Cancel returns empty** |
| 17 | `openFile` | System document picker; the returned path can be read |
| 18 | `saveFile` | Asks for a **file name**, not a location — see below |
| 19 | `selectFolder` | Returns a path with **no picker** — see below |

### Items 18 and 19 are by design — do not report them as bugs

Mobile cannot give an app a filesystem path for a location the user picks, so `saveFile`
asks for a name and returns a path in the app's Documents directory, and `selectFolder`
returns that directory directly. **Do confirm** that a file written there shows up in the
**Files app** under the app's name.

Report as bugs: the picker in item 17 not appearing, a returned path that cannot be read, or
the app hanging after a dialog is dismissed.

### Items 12–14 are regression checks

These passed before and there is new code in their path — the WebView's camera / mic /
location requests are now gated on the requesting origin. If any stops prompting, capture
the origin it saw, not just "camera broke".

### Optional: two dialogs at once

The most likely place a bug is still hiding. From the demo page or the Safari web inspector
attached to the device:

```js
goleo.showMessage({ message: 'first',  buttons: ['OK'] })   // deliberately not awaited
goleo.showMessage({ message: 'second', buttons: ['OK'] })
```

Expected: the first alert appears, the second only after you dismiss it, and both resolve. A
hang, or one that never appears, is a real defect.

---

## 4. If the Swift does not compile

This exact failure happened once already — it is what made 0.10.7 unusable on iOS — and was
fixed in 0.10.8, which CI compiles on a macOS runner before release. So it should not
recur. If it does, for an error like *"type 'GoleoDialogs' does not conform to protocol
'GomobileDialogsProviderProtocol'"*:

1. The generated header is in your own project. Look in
   `Goleo.xcframework/ios-arm64/Goleo.framework/Headers/Gomobile.objc.h` for `openFileJSON`.
   (A successful `goleo build ios` deletes the `.xcframework`; take it from the failed
   build, which is the case you are in.)
2. **Send back those header lines and stop there.** The fix is in the CLI's embedded
   template, which an installed release does not let you edit — it needs a code change and a
   new release on our side.

Optional, if you want to prove a corrected signature rather than just report it: patch
`.goleo/ios/App/AppDelegate.swift` and drive Xcode directly, bypassing goleo so it does not
regenerate the file.

```bash
xcodebuild build -project .goleo/ios/GoleoApp.xcodeproj -scheme App \
  -configuration Debug -sdk iphoneos -destination 'generic/platform=iOS' \
  -allowProvisioningUpdates CONFIGURATION_BUILD_DIR="$PWD"
```

The edit is discarded by your next `goleo build ios`.

Two related notes: `UTType` needs a deployment target of iOS 14 or newer (default is 15.0),
and dialog calls block until answered, so a dialog that never appears hangs that call rather
than timing out.

---

## 5. What to send back

- `goleo version` and the `grep goleo go.mod` line — half the fix lives in each, so a
  failure report is ambiguous without both
- `build-sim.log` and `build-device.log`
- The Xcode console output from the app run (the `UIScene lifecycle` and sandbox lines are
  known-benign; no need to filter them out)
- The checklist with results, and for anything that failed, what you saw rather than what
  you expected
- The `Gomobile.objc.h` dialogs section if section 4 applied

For anything that fails, the single most useful detail is **whether it failed silently or
with an error**.
