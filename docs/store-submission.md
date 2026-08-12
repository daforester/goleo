# Store submission — state, procedures, and what will bite

**Read this before doing anything with the Google Play, Apple, or Microsoft developer
accounts.** It exists because the account work is blocked on things outside the codebase
(account verification, a paid Apple membership), so it will be picked up weeks or months
after the code was written, by someone — or some session — with none of the context.

Everything here is either verified on real hardware or explicitly marked as unverified. Where
a platform requirement changes over time (Play's minimum `targetSdk`, Apple's fees, Console
UI paths), that is flagged rather than asserted — **check the current requirement, do not
trust the number in this file.** Written 2026-08-05 against goleo 0.10.3.

---

## Status at a glance

| Target | Build | Verified on real hardware | Submitted to the store |
|---|---|---|---|
| **Android APK** (debug, sideload) | ✅ | ✅ emulator API 36 — installed, launched, providers round-tripped | n/a |
| **Android AAB** (signed release) | ✅ | ✅ signed APK on emulator: `apkSigningVersion=2`, derived permissions, camera grant→preview | ✅ **accepted — internal track, 2026-08-05** (see below) |
| **iOS Simulator** (`--simulator`) | ✅ | ✅ `macos-14` CI — installed, launched, full bridge working | n/a |
| **iOS device app** (signed, sideload) | ✅ since 2026-08-10 — needs `mobile.ios.development_team` | ✅ **2026-08-11: built by the CLI.** `goleo build ios` (device, signed) and `--simulator` both reached `BUILD SUCCEEDED`, and the signed app ran on an iPhone 17 Pro Max. Supersedes the earlier run, which was driven through Xcode by hand | n/a |
| **iOS `.ipa`** (TestFlight / App Store) | ❌ not implemented | — | ❌ needs a paid Apple account |
| **Windows MSIX** | ✅ | ✅ real `makeappx`, manifest parses, full-trust declared | ❌ **never submitted** |
| **Mac App Store** | ❌ deliberately not built | — | ❌ gated behind an acceptance spike (see below) |
| Windows NSIS / Linux deb+rpm / macOS dmg | ✅ | ✅ NSIS installer verified end-to-end | n/a (direct distribution) |

**The single most useful thing to know:** **Play has accepted a signed AAB** (internal track,
2026-08-05) — so the Android path is proven end to end, store included. That upload immediately
found a defect nothing local could catch (implied `<uses-feature>` entries defaulting to
*required*, filtering the app off devices), which is the point of shipping to a real store rather
than reasoning about it. The Microsoft Store and Apple paths have still **never** had an
artifact accepted, and neither is a code problem — MSIX needs Partner Center identity and a
restricted-capability justification; iOS needs a paid Apple membership.

**iOS device builds changed in 0.10.7** and the table above is worded carefully. Until then
`goleo build ios` could not target a device at all: no way to supply a `DEVELOPMENT_TEAM`, and no
`-destination`, so xcodebuild silently built a *Mac* app. Both are fixed, and a free Apple ID's
personal team is enough to install on your own device (profiles expire after 7 days) — a paid
membership is only needed for `.ipa` distribution. But the fix itself has **not** been run on
hardware; the device evidence in `SPIKES.md` (2026-08-09) came from an Xcode build. Do not
promote that row to ✅ without a CLI device run — `docs/ios-device-verification.md` is the
run sheet for exactly that.

---

## 1. Google Play (Android)

### Status: UPLOADED AND ACCEPTED (internal track, 2026-08-05)

The account was verified and a signed AAB was accepted on the internal testing track under a
throwaway package name. **This is the first time any goleo artifact has been accepted by a
store**, so the entries below are observed facts rather than expectations.

**Play's report on the accepted bundle (goleo 0.10.4, versionCode 101, versionName 0.1.1):**

- **19 permissions** — every one reconciled: goleo's 16 (the 14 the build prints, plus
  `BLUETOOTH`/`BLUETOOTH_ADMIN` carrying `maxSdkVersion="30"`, which the build deliberately does
  not print), 2 merged in by AndroidX (`RECEIVE_BOOT_COMPLETED` from WorkManager,
  `<package>.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION` from androidx.core), and
  `com.android.vending.CHECK_LICENSE` added by **Play itself** — verified absent from the merged
  manifest, so it is not goleo's. `ACCESS_COARSE_LOCATION` is correctly absent: the platform adds
  that at install time, not in the manifest.
- **7 features** — `bluetooth`, `bluetooth_le`, `camera`, `faketouch`, `location`,
  `location.gps`, `nfc`. Six are goleo's explicit `required="false"` declarations; `faketouch` is
  implied by the platform and left alone (every device satisfies it).
- **19,041 supported Android devices** out of a catalogue of roughly 20k models.

**The first upload (versionCode 100) exposed a real defect — see `SPIKES.md`, 2026-08-05.** It
reported 6 features where goleo declared 3, because aapt2 *implies* a `<uses-feature>` from
certain permissions and an implied entry defaults to `required="true"` — so
`BLUETOOTH`/`BLUETOOTH_ADMIN` implied a **required** `android.hardware.bluetooth` and
`ACCESS_FINE_LOCATION` a **required** `android.hardware.location`, both hard device filters on an
app that degrades gracefully. Fixed in 0.10.4; the accepted bundle above is the corrected one.

**Honest limitation on the device figure: there is no before/after.** The supported-device count
for versionCode 100 was never captured, so 19,041 does not by itself demonstrate the fix widened
reach — it is consistent with the fix having worked *and* with it having made no measurable
difference on this particular permission set. Requiring `bluetooth` and `location` excludes only
models with no Bluetooth radio and no location provider at all, which is a thin slice of the
catalogue, so the true delta was expected to be small (a few hundred devices at most).

**That gap was deliberately left open — do not chase it.** The pre-fix bundle's count is still
readable (*Bundle explorer → versionCode 100 → device compatibility*) if anyone ever wants the
comparison, but the decision on 2026-08-05 was that it is not worth the trip: the expected delta
is a few hundred devices at most, the fix is verified at the manifest level, and a measurement
that could plausibly come back as "no change" would tell you nothing you do not already know.

Either way the fix stands on **correctness, not on this number**: the same defect on a more
commonly-absent implied feature would have excluded a large fraction of the catalogue, and there
would have been no local symptom then either.

### What was already proven before the upload

On a real Android 36 x86_64 emulator, from a signed release APK built by the demo scaffold
(full detail in `SPIKES.md`, "Phase 4"):

- `apkSigningVersion=2` with a real signature block — the signing path works end to end:
  `build.gradle.kts` reads credentials from the environment, Gradle applies the
  `signingConfig`, and the artifact is signed rather than the `-unsigned` variant.
- `versionCode=100`, `versionName=0.1.0` — both were hardcoded `1`/`"1.0"` before Phase 4, so
  `goleo.json`'s values were loaded and thrown away. `0.1.0 → 100` is the derived
  `major*10000 + minor*100 + patch`.
- Exactly the 14 permissions the demo enables, each attributable to a feature. A minimal
  scaffold gets 3.
- `BLUETOOTH`/`BLUETOOTH_ADMIN` are in the generated manifest but **absent from the installed
  package** on API 36 — the `maxSdkVersion=30` bound works, which is precisely the
  "unnecessary permission" report it exists to avoid.
- The full runtime chain: derived manifest permission → Android prompt → grant →
  `WebChromeClient.onPermissionRequest` → `getUserMedia` → live camera frames.

CI (`mobile-verify`'s `android-release` job) additionally covers the AAB build, `jarsigner`
verification, the manifest being minimal, and that `--no-sign` really produces something
unsigned.

### Procedure

**Step 1 — pick a throwaway package name.** A package name on Play is **permanent**: it
identifies the listing, cannot be changed, and cannot be reused even after unpublishing. So do
**not** spend a name in a real namespace on a test upload. Random letters are fine; goleo
validates the shape (`cli/cmd/android_package.go` — two segments minimum, no Java keywords, no
leading digits per segment):

```jsonc
// goleo.json
"mobile": { "android": { "package_name": "qvbnxwtz.rmpldskg" } }
```

**Step 2 — build a signed AAB.**

```bash
goleo generate android-key      # writes release.jks, prints the four env vars
# set GOLEO_ANDROID_KEYSTORE / _KEYSTORE_PASSWORD / _KEY_ALIAS / _KEY_PASSWORD
goleo build android --release   # -> app.aab
```

Note the permission list the build prints — that is what you are about to check Play against.
`--release` **errors** without a keystore rather than producing an unsigned artifact;
`--no-sign` is the explicit way to build one anyway.

**Step 3 — upload.** Play Console → *All apps* → *Create app* → *Testing* → *Internal testing*
→ *Create new release* → upload `app.aab`. Rollout is blocked until *App content* is complete:
app access, ads declaration, content rating, target audience, data safety, privacy policy URL.
Fewer items than a production release, but not zero — budget an hour for the questionnaires.

**Step 4 — testers.** *Testers* tab → add an email list → *Roll out*. Install through the
opt-in URL it gives you.

### What you are actually checking

This upload is not a formality; it answers two things nothing else can:

1. **Release → Bundle explorer → Permissions.** Does the list match what the build printed, and
   does Play flag any as unjustified? The whole point of the derived-permissions work is that
   the old static manifest declared 13 permissions for every app, which is exactly what Play
   objects to. If Play still complains, the derivation needs narrowing.
2. **Does Play accept the `targetSdk` and `versionCode`?** goleo's template defaults are
   `minSdk 24` / `targetSdk 36`. Play raises its minimum `targetSdk` annually — **check the
   current requirement**, do not assume 36 is still acceptable.

### Gotchas, all learned the hard way

- **`keytool` is not on `PATH`** even when `JAVA_HOME` is right — it lives in the JDK's `bin/`.
  That is why `goleo generate android-key` exists; it uses the JDK goleo already resolves for
  Gradle.
- **A trailing space in `GOLEO_ANDROID_KEYSTORE` fails 37 seconds into the build** at
  `:app:packageRelease`, as `Trailing char < > at index 141` — naming neither the variable nor
  the space. `set VAR=path ` in cmd.exe keeps that space. goleo trims it now and checks the
  path exists up front, but if you see an unexplained Gradle failure, look for whitespace.
- **Play rejects a duplicate `versionCode`.** On a re-upload, bump with
  `GOLEO_ANDROID_VERSION_CODE=<n>` (CI wins over `goleo.json`) rather than editing the file.
- **Play App Signing:** you upload with your *upload key*; Google re-signs with the *app signing
  key* it holds. So the signature on the installed app is not the one `jarsigner` shows locally.
  Losing the upload key is recoverable (Google can reset it); this is a reason not to treat the
  throwaway keystore as precious.
- **Device-side permissions will not match the 14 exactly**, and that is correct.
  `RECEIVE_BOOT_COMPLETED`, `BIND_JOB_SERVICE` and `DUMP` arrive via AndroidX (WorkManager)
  manifest merging, and the platform adds `ACCESS_COARSE_LOCATION` when an app requests
  `ACCESS_FINE_LOCATION`. Verified by diffing the generated manifest against
  `intermediates/merged_manifest/release/.../AndroidManifest.xml` against the installed package
  — nothing leaks from goleo's derivation.

---

## 2. Apple (iOS)

### Tier 1 — done, and it needs no account

`goleo build ios --simulator` works and is verified on a `macos-14` runner from a freshly
scaffolded project: `GoleoApp.app` built, installed on a simulator, launched, and still running
— with the screenshot showing the embedded UI rendered, `goleo:getOS` answered by the Go
backend (`"os":"ios","arch":"arm64"`), a custom `invoke()` returning, and `heartbeat` push
events arriving. So the gomobile-hosted Go backend, loopback asset serving, WKWebView, bridge
invoke and event push all work on iOS. `mobile-verify`'s `ios-simulator` job gates this.

**This took six defects to reach, each hidden behind the one before it.** Read the 2026-08-04/05
entries in `SPIKES.md` before touching the iOS path — particularly the naming rule, which is
not guessable from the source:

> The Swift **module** is `Goleo` (gomobile titlecases the `-o` basename), but every **symbol**
> carries the titlecased Go **package** name, `Gomobile`. So `import Goleo` plus
> `GomobileSetHomeDir(...)`. Package-level Go funcs become **C functions** and take no argument
> labels. Each Go interface generates a protocol *and* a same-named class, so Swift appends
> `Protocol`.

`mobile-verify` prints the generated `Gomobile.objc.h` on every run — that is the ground truth
if the bindings ever drift, and reading it beats guessing across CI round-trips.

### Tier 2 — blocked on a paid account, NOT on Mac hardware

Everything below is reachable from a GitHub `macos-14` runner. What it needs is Apple Developer
Program membership (annual fee; **check the current price and terms**) for a signing
certificate and provisioning profile. A runner cannot substitute for that.

`goleo build ios --release` currently **refuses**, with the reason and the two alternatives, so
nobody gets a debug build labelled as a release.

**What is missing from the codebase** (nothing exists for these yet):

- `cli/cmd/templates/ios/ExportOptions.plist.tmpl` — `method` (`app-store`/`ad-hoc`/
  `development`), `teamID`, `signingStyle`, `provisioningProfiles` map.
- `cli/cmd/templates/ios/App/app.entitlements.tmpl` — whatever the app actually needs; keep it
  minimal, every entitlement is a review question.
- `xcodegen.yml` additions: `CODE_SIGN_ENTITLEMENTS`, `DEVELOPMENT_TEAM`, `CODE_SIGN_STYLE`.
- `buildForIOS`: replace `xcodebuild build` with `archive` + `-exportArchive
  -exportOptionsPlist`, producing a `.ipa`. Keep the artifact verification that is already
  there — it exists because the CLI used to print a path it never wrote.
- Upload: `xcrun altool --upload-app` (or `notarytool`, depending on what Apple still accepts).
  Put it in `publish`, not `build` — the same separation `--publish` already uses.

**Gotchas to expect:**

- **The bundle identifier is effectively permanent** once a listing exists in App Store Connect,
  same hazard as the Play package name. Use a throwaway for a first submission.
- `mobile.ios.bundle_identifier` **is** honoured now (it was dead config until 0.10.2). Setting
  it to something other than the Android `package_name` used to crash the app on launch, because
  the `BGTaskScheduler` identifier came from the Android name while `Info.plist` permits the iOS
  one. Fixed in 0.10.3 — but it is the shape of bug to watch for: **two identifiers that used to
  be the same value are now independent.**
- The generated Xcode project is pinned to `projectFormat: xcode14_0` (objectVersion 56) so any
  Xcode from 14 up can open it. XcodeGen's default writes objectVersion 77, which no Xcode below
  16.0 will open — and an *unrecognised* `projectFormat` value silently falls back to that
  default. Do not "modernise" this without a reason.
- `UIRequiredDeviceCapabilities` includes `arm64`. Fine on Apple Silicon and on devices; if
  anyone ever runs a simulator on an Intel Mac, check this first.

---

## 3. Microsoft Store (MSIX)

**Built and verified; never submitted.** `goleo build windows --bundle --windows-format msix`
produces a real `.msix` — verified with actual `makeappx`, manifest parses, declaring
`Windows.FullTrustApplication` plus the restricted `runFullTrust` capability. Full trust is what
keeps the loopback bridge working: a full-trust package is not sandboxed.

To submit you need **Partner Center identity**, and goleo validates it rather than guessing,
because a wrong value builds and signs happily then fails at install or submission with nothing
pointing at the cause:

```jsonc
// goleo.json
"windows": { "msix": {
  "identity_name": "12345Acme.MyApp",              // EXACTLY as Partner Center shows it
  "publisher": "CN=Acme Ltd, O=Acme Ltd, C=GB",    // the CERTIFICATE SUBJECT, not a company name
  "publisher_display_name": "Acme Ltd"
}}
```

`runFullTrust` is a **restricted capability**: Microsoft must approve its use, which is a
question on the submission. Expect to justify "the app runs a local HTTP/WebSocket server for
its own UI". This is the one part of the MSIX path that could actually be refused, and it has
never been tested against a real reviewer.

Version: MSIX reserves the fourth part, so goleo forces revision `0` and refuses a non-zero
one.

---

## 4. Mac App Store — deliberately not built

**Do not build MAS tooling before running the acceptance spike.** The App Sandbox collides with
goleo's architecture in ways that may make it non-viable:

- the **self-replacing updater is outright forbidden**;
- arbitrary filesystem access is gone (the FS scope model helps, but the sandbox is stricter);
- `exec.Command` in share/openURL breaks;
- autostart login items change shape;
- the loopback WebSocket needs the `network.server` entitlement and is a review risk.

The spike is: get a **sandboxed hello-world with the loopback bridge accepted** by review. Only
if that passes is there any point building a `--mac-appstore` profile (excludes updater/autostart
/shell tags, adds entitlements + embedded provisioning profile, `productbuild`/`productsign`,
`altool --validate`). Building the tooling first risks weeks of work on a path Apple rejects.

---

## Appendix — things that will save you real time

**Runner cost.** macOS runners bill at **10×**. A successful `ios-simulator` run is ~300s ≈ 51
billable minutes; booting a simulator and launching is 118s of that on its own. A run that fails
at the build is ~150s. If iOS needs iterating again, do it on a `spike/ios-*` branch with a
single-job workflow (the pattern is recorded in the 2026-08-05 `SPIKES.md` entry) rather than on
master, which also triggers `ci` and both Android jobs. **And do not estimate iteration cost
from a failing run — it skips the expensive steps.**

**Determine names from source, not from trial.** Two costly questions were settled by reading
upstream instead of pushing: gomobile's framework/module naming (`cmd/gomobile/bind_iosapp.go`)
and XcodeGen's `projectFormat` → objectVersion mapping (`Sources/XcodeGenKit/ProjectFormat.swift`).
Each would otherwise have been a CI round-trip, or several.

**Mutation-test every claim, and make the mutation assert it applied.** A mutation whose pattern
does not match silently changes nothing, the test passes, and that reads as "the test does not
catch this" — the inverse of the truth. `cli/cmd/android_deps.go` is CRLF while most of
`cli/cmd` is LF, so always detect the file's line ending and `assert count == 1` before running
the test.

**Silent truncation reads as complete coverage.** A `head -80` on a generated-header dump cut an
alphabetical list after the fourth entry, which looked exactly like "there are only four" and
would have produced five wrong Swift conformances. Print the total before truncating.

**A `{{...}}` in a template comment is still an action.** Every file under
`cli/cmd/templates/{android,android-dev,ios}` goes through `text/template`, so a literal
`{{if ...}}` in a YAML or XML comment breaks the file. `TestEveryMobileTemplateParses` covers
this now.

**The dev path hides the user path.** `GOLEO_ROOT` and `linkBridge()` mean a dev scaffold
resolves neither the Go module version from the proxy nor the npm range from the registry, which
is how three user-visible packaging bugs shipped. `release-smoke.yml` exists to catch that: it
installs the published CLI with **no `actions/checkout` at all** and runs after publishing. If
you change anything about packaging, that workflow is the one that matters.
