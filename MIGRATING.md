# Migrating between goleo versions

Only changes that can break a working app are listed here. goleo is pre-1.0, so
breaking changes do happen — each one below says how to tell whether it affects
you and what to do.

---

## 0.9.0 — filesystem access is confined by default

**Affects:** apps that call `runtime.RegisterFS` (which `RegisterDesktopFeatures`
does, so both scaffolds enable it) **and** pass absolute paths from outside the
app's own data directory.

### What changed

`runtime/fs` previously rejected only *relative* traversal (`../x`). Every
absolute path was allowed, so anything running in the webview — including an XSS,
a compromised npm/CDN dependency, or a third-party analytics script — could read
`~/.ssh/id_rsa` or hand a home directory to `os.RemoveAll` via `goleo:fsDelete`.
`Policy.FSRoots` looked like the mitigation but had no call sites at all.

The `fs` plugin is now confined to:

1. the app's own data directory (`os.UserConfigDir()/<AppID or Title>`),
2. anything in `Policy.FSRoots`,
3. any path the user picked in a native file dialog during this session.

| Operation | Outside the scope |
|---|---|
| write, delete | **refused** — no compatibility window for `os.RemoveAll` |
| read | allowed, logs a one-time deprecation warning; **will become an error** |
| write to a system location (`C:\Windows`, `/usr`, `/etc`, …) | **refused in every mode** |

Symlinks are resolved before the check, so a link inside an allowed root cannot
point out of it.

### Do I need to change anything?

Run your app and exercise its file features. If it works with no
`goleo: DEPRECATED — read outside the allowed filesystem roots` lines in the
output, you are unaffected.

- **Reading a file the user picked** — no change needed. Dialog results are
  granted automatically.
- **Reading/writing your own app data** — no change needed.
- **A fixed directory outside app data** (a shared data dir, a sibling project):
  add it to the policy.

  ```go
  app.SetPolicy(&runtime.Policy{
      Allow:   []string{"goleo:fs*"},
      FSRoots: []string{"/srv/shared-data"},
  })
  ```

  `FSRoots` widens the scope; it does not replace the app data directory.

- **You genuinely need the whole disk** (a file manager, a dev tool):

  ```go
  runtime.New(runtime.Config{FSScope: runtime.FSScopeUnrestricted})
  ```

  or set `GOLEO_FS_UNRESTRICTED=1` in the environment. The system-location
  deny-list still applies to writes.

### What is already in scope for you

- **`appDataDir()` brings its own directory into scope.** Calling
  `goleo:fsAppDataDir` / `appDataDir("my-app")` registers the directory it returns,
  so you can write there immediately without touching `Policy.FSRoots`. Vending a
  path and then refusing writes to it would be incoherent — and it broke the
  scaffolded demo, whose FileSystem page does exactly this.

  One new restriction: `appName` becomes a path element, so it may not contain a
  path separator or `..`. `appDataDir("../../etc")` used to return `/etc`; it now
  errors. Without that guard, granting the result would have handed out an
  arbitrary directory to anything running in the webview.

- **`homeDir()` grants nothing.** It answers "where is home", which is
  informational. Granting it would hand back the whole user profile and defeat the
  confinement, so writing under it still needs `Policy.FSRoots` or a dialog-picked
  path.

- **Relative paths resolve against the backend's working directory**, not your app
  data directory, and are therefore usually out of scope. Previously a relative path
  silently wrote into the project directory. Resolve `appDataDir()` first and join
  onto it.

### Also in this change

- `Policy.FSRoots` is now actually enforced (it previously did nothing).
- Error messages are preserved end-to-end. `@goleo/bridge`'s `fs` wrappers used to
  replace every failure with `"<op> requires the Go backend"`, which would have
  masked the confinement message entirely. They now rethrow the backend's own error
  — which names the offending path and the three ways to allow it — and only claim
  the backend is missing when it genuinely is.
- `Policy.HTTPHosts` and `Policy.ShellPrograms` are documented as **reserved** —
  goleo has no http or shell plugin, so they gate nothing today. They are still
  accepted so a policy written now keeps working when those plugins land.
- `Policy.AllowsFSPath` is retained as a raw helper but is **not** the enforcement
  path; its "empty list means unconstrained" rule is the opposite of the default
  the plugin needs. Use the scope model, not this helper, to decide access.

---

## 0.9.0 — `@goleo/bridge` reports failures instead of inventing values

**Affects:** any code that relied on a bridge call *succeeding* when it had actually
failed. If your callers already `try/catch` (or `.catch()`), you are unaffected.

Several wrappers converted a failure into a value the caller could not distinguish
from a real result. Each now throws when the backend is present and only falls back
when there genuinely is no backend.

| Call | Was, on failure | Now |
|---|---|---|
| `showMessage()` | returned `'OK'` | throws; with no backend, asks via `confirm`/`alert` |
| `saveFile()`, `selectFolder()` | returned `null` (= "user cancelled") | throws; `null` still means cancel |
| `showPrompt()` | silently opened a browser prompt | throws; browser prompt only with no backend |
| `openFile()`, `openFiles()` | **never settled** if the picker was cancelled | resolves `null` / `[]` |
| `readTextFile()` etc. | `"<op> requires the Go backend"` | the backend's real error |
| `storeGet()`/`storeSet()` etc. | silently switched to `localStorage` | throws; `localStorage` only with no backend |
| `bleConnect()` | resolved having done nothing | throws |

The one to look at is **`showMessage`**. It used to return `'OK'` when the dialog
failed, so a caller asking "Delete all data?" proceeded with the deletion. If you
wrote code assuming it always resolves, it can now reject — which is the point.

Two additions rather than changes:

- **`reconnect()`** — there was previously no way out of local-only mode. If the
  initial connect timed out (3s default) or the retries were spent, every non-core
  method threw "backend not connected" for the rest of the session, even after the
  backend came up. Wire it to a retry affordance or to `bridge:reconnectFailed`.
  Retries now also back off exponentially with jitter instead of a fixed 3s.
- **`Capabilities.menu`** — the Go side has always returned it; the TypeScript type
  omitted it. `menuSupported()` now shares the cached, no-backend-safe path with
  `isWindowingSupported()`/`isTraySupported()` instead of throwing where they
  return `false`.

### Binary files cross the bridge as base64

`readBinaryFile()`/`writeBinaryFile()` were corrupting any payload that was not
plain ASCII, in both directions. They now use base64 on both ends, which is what
`encoding/json` already does for a Go `[]byte`.

You only need to act if you call `goleo:fsReadBinaryFile` / `goleo:fsWriteBinaryFile`
through `invoke()` **directly** rather than through the wrappers — in which case
`data` is now base64 in both directions. The wrappers still take and return a
`Uint8Array`, so their callers change nothing.

---

## 0.9.0 — each app gets its own key/value store

**Affects:** apps using `RegisterStore` / `@goleo/bridge`'s `storeGet`/`storeSet`.
**No action needed** — existing data is migrated automatically. Read on only if you
care what happens.

Every goleo app on a machine previously shared **one** `store.json`, under
`<UserConfigDir>/goleo-app/`. Two different goleo apps read and clobbered each
other's keys. The store now lives under the app's own name
(`Config.AppID`, falling back to `Config.Title`).

On first run at the new path, the old shared store is **copied** into it, so nothing
is lost. It is a copy rather than a move because other apps on the machine may still
be reading the legacy file. An app that already has its own store is never
overwritten.

Two consequences worth knowing:

- **You may inherit another app's keys once.** If several of your goleo apps shared
  the legacy store, each one adopts a copy of the whole thing on first run. Delete
  any keys that do not belong to that app, or clear them with `storeClear()`.
- **The legacy file is left behind.** Once every app on the machine has migrated,
  `<UserConfigDir>/goleo-app/store.json` can be deleted by hand.

Also fixed here: writes now `fsync` before the rename (a crash could previously
leave a zero-length store where a valid one had been), and each write uses a unique
temp file — the old fixed `store.json.tmp` meant two writers, such as a second
instance racing the primary, shared one temp path and could interleave into a
corrupt store.

---

## 0.9.0 — `goleo:openURL` only opens web schemes

**Affects:** apps that pass anything other than `http`, `https`, `mailto` or `tel`
to `openURL` / `goleo:openURL`.

`openURL` handed any string to the OS handler, so `file://`, UNC paths and paths
to executables all went through — arbitrary execution from anything running in the
webview. It is now restricted to `http`/`https`/`mailto`/`tel` plus the app's own
`Config.URLScheme` (registered automatically by `App.Run`).

To open a different scheme deliberately, register it:

```go
runtime.AllowURLScheme("myproto")
```

Opening a local file is intentionally not supported through `openURL` — use the
`fs` plugin (within its scope) or a native dialog.

---

## 0.9.0 — dev-mode bridge rejects public origins

**Affects:** development setups served from a public hostname (a tunnel such as
ngrok, or a hosted preview) — not local Vite, not `goleo emulate android`, not LAN
device testing.

In dev mode the bridge accepted **any** `Origin`, so any page the user happened to
visit could open `ws://127.0.0.1:<port>/ws`, invoke every registered method
(including the filesystem plugin) and read the replies. Dev now allows loopback,
private-network and link-local origins on any port — which covers Vite on any
port, the Android emulator's `10.0.2.2`, and real-device testing over the LAN —
and rejects public origins with an explanatory log line.

If you serve your dev frontend from a public hostname:

```bash
GOLEO_DEV_ALLOWED_ORIGINS=https://my-tunnel.example.com goleo dev
```

Comma-separate multiple origins. Production behaviour is unchanged.

---

## 0.9.0 — a malformed `goleo.json` now fails the build

**Affects:** projects whose `goleo.json` does not parse, or has a key of the wrong
type (`"version": 2.0` instead of `"2.0"`).

Every loader used to swallow parse errors and fall back to defaults, so a trailing
comma produced a *successful* build of an app named "Goleo App" with identifier
`com.goleo.app` and version `0.1.0`. Malformed config is now reported:

```
goleo.json is not valid JSON: invalid character ',' looking for beginning of object key string
```

Fix the JSON. Unknown/extra keys are still accepted.

Three previously-ignored keys now take effect —
`mobile.android.min_sdk`, `mobile.ios.deployment_target` and
`mobile.ios.bundle_identifier`. If you had set them expecting them to work, they
now do; if you set them to something wrong, the build will change accordingly. The
iOS bundle identifier previously fell back to the *Android* `package_name`, and
still does when unset.

> **Correction (0.10.2):** that was only true of `min_sdk`. The two iOS keys were
> read out of `goleo.json` in 0.9.0 but then discarded — `xcodegen.yml` still took
> the bundle id from the Android `package_name` and hardcoded the deployment
> target. They genuinely take effect from 0.10.2; see that entry below.

---

## 0.9.0 — existing projects must update their `@goleo/bridge` pin

**Affects:** every project scaffolded by `goleo new` before this release. Check with:

```
grep '@goleo/bridge' frontend/package.json
```

If it says `"^0.2.1"`, you are affected. `goleo dev` and `goleo build` now warn
about it too.

**What was wrong.** The generated `frontend/package.json` hardcoded
`"@goleo/bridge": "^0.2.1"`. A caret on a `0.x` version locks the *minor*, so that
range resolves to **0.2.9** — it never picks up 0.3 or later. The Go side had
already been changed to pin the CLI's own version, so a new project got a v0.8.x
runtime alongside a bridge six minors old.

That is a skew across a wire contract, not just an old dependency, and it fails
quietly:

- **binary file I/O was broken.** `writeBinaryFile` in 0.2.x sends
  `TextDecoder` output where the current runtime expects base64, and
  `readBinaryFile` expects a shape the runtime no longer returns.
- **filesystem errors were misattributed.** Confinement errors surfaced as
  `"… requires the Go backend"` instead of the real message naming the path.
- Missing since 0.2.x: `reconnect()` (a slow backend stranded the app in
  local-only mode for the whole session), `showMessage` throwing rather than
  reporting a fake `'OK'`, per-call localStorage fallbacks in `store`, and the
  cached-capabilities fix that made `openWindow` throw "not supported" forever.

Nobody hit this while working on goleo itself because `goleo new` npm-links a local
bridge checkout over the dependency — the development path masked it and only
end users got the stale pin.

**Fix — in your project:**

```
cd frontend && npm install @goleo/bridge@<your goleo version>
```

Use the version `goleo version` reports, so the bridge and runtime stay in
lockstep. New projects now get this automatically: the pin is injected from the
CLI's own version, exactly like the `go.mod` require.

---

## 0.9.0 — `goleo build` on Windows now produces `app.exe`

**Affects:** Windows only, and only scripts that referred to the built file by name.

`goleo build` (the default `current` target) wrote a binary with **no extension** —
`app`, not `app.exe`. Windows will not execute that: double-clicking does nothing
and `Start-Process .\app` fails with "the system cannot find all the information
required". Only the explicit cross-target `goleo build windows` was correct, because
the `current` entry in the target table took the host's `GOOS` while hardcoding an
empty extension.

**What to do.** Nothing, unless a script, installer input, or CI step referenced
`app` literally on Windows — those need `app.exe`. `-o` is unaffected: an explicit
`-o myapp` becomes `myapp.exe`, and `-o myapp.exe` is left alone (no doubling).

If you had worked around this by renaming the output yourself, drop the rename.

---

## 0.9.1 — `goleo:share` only accepts web URLs

**Affects:** apps calling `runtime.RegisterShare` (the demo scaffold does) that passed
`goleo:share` anything other than an `http`/`https`/`mailto`/`tel` URL or their own
registered `Config.URLScheme`.

On desktop, share hands `data.URL` to the OS default handler — `rundll32
url.dll,FileProtocolHandler` on Windows, `open` on macOS, `xdg-open` on Linux. It did
so without validating the URL, so a `file://` URL, a UNC path, or a bare path to an
executable was **arbitrary execution from any script in the webview**:

```js
invoke('goleo:share', { url: 'file:///C:/Windows/System32/calc.exe' })
```

`goleo:openURL` was given a scheme allow-list for exactly this reason. Share reaches
the same handlers and was missed — it was written from the same mechanism but not the
same guard. Both now go through one validator, so a third caller cannot diverge again.

**What to do.** Nothing for ordinary use: sharing a link still works. If you were
relying on share to open a local file, use the filesystem plugin, or register your
own scheme via `Config.URLScheme` and handle it in-app.

---

## 0.9.1 — Linux notifications and the Windows prompt handle text more faithfully

**Affects:** Linux desktop notifications, and multi-line messages in
`showPrompt` on Windows. No API change.

`goleo:notify` is a default builtin, so its title and body come from the frontend.
On Linux they were passed to `notify-send` as bare positional arguments, and
notify-send uses GLib option parsing — so a title beginning with `--` was read as an
option rather than as the summary. `--help` or `--version` made notify-send print and
exit 0, meaning **the notification silently never appeared while the call reported
success**, and `--icon=` / `--hint=` / `--expire-time=` let a frontend restyle or
suppress notifications the app believed it controlled. Arguments are now passed after
a `--` terminator. No shell was involved, so this was flag confusion, not RCE.

Separately, `showPrompt` on Windows built its PowerShell script by replacing newlines
with a backtick-`n` escape. That escape only means anything in a *double*-quoted
PowerShell string; the values are interpolated into *single*-quoted literals, where it
is taken literally — so every newline in a title or message reached the dialog as the
two characters `` `n ``. Newlines are now preserved, and `
` normalises to one
line break instead of two.

**What to do.** Nothing. If you had worked around the Windows prompt by pre-formatting
messages without newlines, you can stop.

---

## 0.10.0 — Android permissions are derived from the features you enable

**Affects:** every Android build. Check the permission list `goleo build android` now prints
before you ship.

The generated `AndroidManifest.xml` used to declare thirteen permissions for **every** app —
`CAMERA`, `RECORD_AUDIO`, both location grants, `VIBRATE`, `NFC` and four `BLUETOOTH_*` —
because the bundled demo needed them. Play flags unjustified permissions, and a user
installing a note-taking app should not be told it wants their camera and location.

They are now derived from the features your app enables, plus an unconditional core set
(`INTERNET`, `ACCESS_NETWORK_STATE`, `POST_NOTIFICATIONS`). A minimal app declares 3
instead of 13+; the demo scaffold declares 14, each traceable to a feature it uses.

**This can remove a permission you were relying on.** Detection looks for `Register*` calls
in your Go source. If you use a capability purely through a frontend browser API — say
`getUserMedia` in your Vue code without ever calling `runtime.RegisterCamera` — nothing in
your Go source names it, so the permission is no longer declared. On Android a missing
manifest permission fails **silently**: the runtime request returns "denied" without
prompting.

**What to do.** Run a build and read the printed list:

```
  Manifest permissions (3):
    ACCESS_NETWORK_STATE     <- core
    INTERNET                 <- core
    POST_NOTIFICATIONS       <- core
```

Anything missing goes in `goleo.json`, which is trusted verbatim:

```json
{
  "mobile": {
    "android": {
      "extra_permissions": ["RECORD_AUDIO", "android.permission.READ_CONTACTS"]
    }
  }
}
```

Bare names and fully-qualified ones both work.

**Also in this release**, for the same target:

- `goleo build android --release` produces a **signed `.aab`** for Play. It is an error if no
  keystore is configured — an unsigned release artifact cannot be uploaded or installed —
  and `--no-sign` is the explicit way to build one anyway. `--android-format apk` gives a
  signed APK for distribution outside a store. Without `--release` the output is an
  unsigned debug `.apk`, exactly as before.
- `goleo generate android-key` creates a keystore using the JDK goleo already resolves, so
  it works when `keytool` is not on your PATH (it usually is not — it lives in the JDK's
  `bin/`).
- `versionCode` and `versionName` now come from `goleo.json` instead of being hardcoded `1`
  and `"1.0"`. If you were relying on every build declaring version 1, Play will now see
  the real version. `GOLEO_ANDROID_VERSION_CODE` overrides it for CI, then
  `--version-code`, then `mobile.android.version_code`, then a value derived from your
  semver (`1.2.3` becomes `10203`).
- `mobile.android.min_sdk` and `target_sdk` are finally read. They were loaded from
  `goleo.json` and then discarded, so the template's 24 and 36 always won.
- `--bundle` and `--publish` are now **refused** on mobile and pwa targets instead of being
  silently ignored. Neither had a meaning there: the APK/AAB is already the distributable,
  and mobile apps update through the store rather than goleo's self-updater.


---

## 0.10.2 — the iOS build honours your `goleo.json`

`mobile.ios.bundle_identifier` and `mobile.ios.deployment_target` now reach the generated
Xcode project. Until this release they were parsed and then discarded: the bundle id came
from the **Android** `package_name` and the deployment target was hardcoded to 15.0. (The
0.9.0 note above claimed otherwise; that was wrong for these two keys.)

**What changes for you.** If you set `mobile.ios.bundle_identifier`, your next iOS build
carries that identifier instead of the Android one. To iOS, TestFlight and the App Store a
bundle id *is* the app's identity, so this is the same app under a new name: an already-installed
build will not upgrade in place, and an App Store record registered under the Android id will
not accept the new build. If you want the old behaviour, set `bundle_identifier` to your
`package_name` explicitly. Leaving it unset still falls back to `package_name`, so a project
that never set it is unaffected.

`CFBundleName` and `CFBundleDisplayName` now come from your app name rather than being
hardcoded "Goleo App", and `CFBundleShortVersionString`/`CFBundleVersion` come from the project
`version` rather than always being `1.0`/`1`. If you were relying on every build reporting
version 1.0, it now reports the real one.

**`--ios-target` and `--android-api` no longer have version defaults of their own.** They used
to default to iOS 14.0 / API 24 and drive gomobile, while the *project's* minimum came from
`goleo.json` — two independent sources that could disagree, and on iOS did so out of the box
(14.0 versus 15.0). A Go library built for a *higher* minimum than the app fails to link, so
lowering `mobile.ios.deployment_target` below 14.0 used to produce an error naming a version
you never set. Both flags now default to the config value and exist only as a per-build
override. If you were passing `--ios-target`/`--android-api` to *raise* the minimum, move that
value into `mobile.ios.deployment_target` / `mobile.android.min_sdk` so the native project
agrees with it.

**Also in this release:** `goleo build ios --simulator` is added — a build against the iOS
Simulator SDK with code signing off, which is the only iOS target needing no Apple Developer
account. **It did not complete in 0.10.2**, and neither did `goleo build ios`: see
[Mobile](docs/guide/10-mobile.md). What this release fixes is everything up to the Swift
compile, none of which worked before because iOS had never been built anywhere:

- gomobile now binds a simulator slice (`-target ios,iossimulator`), confirmed on a macOS runner
- the generated Xcode project is in a format your Xcode can open (it was XcodeGen's default,
  objectVersion 77, which no Xcode below 16.0 will read)
- the project and the build agree on the framework name (`Goleo.xcframework`; the project asked
  for `App.xcframework`, which never existed)
- `Info.plist` no longer hardcodes the app name and version, and the `LaunchScreen` storyboard
  it references is actually present — it was missing from the template altogether, which is a
  black launch screen and an App Store rejection
- `mobile.ios.bundle_identifier` and `deployment_target` reach the project (see above)

What remains is `App/AppDelegate.swift`: its gomobile binding names were written without ever
being compiled, and the Swift module name (`Goleo`, from the artifact) differs from the
Objective-C class prefix gobind derives from the Go package (`Gomobile`). That is being fixed
against the real generated headers rather than guessed at.

**Flags that were accepted and ignored are now refused or honoured.** Each did nothing,
so a script passing one already was not getting what it asked for; the change is that you
now find out.

| Flag | Was | Now |
|---|---|---|
| `goleo build ios --release` | accepted, ignored — you got a debug build and a success message | refused, and says there is no `.ipa` path yet |
| `--android-abi` on a non-Android target | accepted, ignored | refused (`--arch` is the desktop equivalent) |
| `--android-ndk` | accepted, ignored **everywhere** — resolution went straight to `ANDROID_NDK_HOME` and autodiscovery | honoured, ahead of the environment; an error if the path is not an NDK |
| `goleo emulate android -o NAME` | accepted, ignored | removed — the dev APK is installed out of `.goleo/` and never copied, so there was nothing to name |

If you were passing `--android-ndk` and relying on `ANDROID_NDK_HOME` winning, the flag now
takes precedence. If it pointed somewhere stale, the build stops rather than quietly using a
different NDK.

**`goleo emulate android` now honours `mobile.android.min_sdk` / `target_sdk`.** The dev
Android project hardcoded `compileSdk 36 / minSdk 24 / targetSdk 36` while release builds
read the config. Raising `min_sdk` above 24 used to fail only under `goleo emulate` (Gradle
rejects a library whose `minSdk` exceeds the app's); lowering it meant dev ran on devices the
release build did not support. The dev build's launcher label is now your app name rather
than a shared "Goleo (Dev)", and its `versionName` carries your real version with a `-dev`
suffix.

**Mobile artifacts got smaller.** The frontend was copied into the native project
(`app/src/main/assets`, `App/Assets`) *as well as* being embedded in the Go library, which
is the copy the WebView actually loads over loopback. Nothing read the native copy, so every
APK, AAB and `.app` carried the whole frontend twice. If you added code that reads
`file:///android_asset/…` or a bundle resource, it will no longer find those files — load
over `http://127.0.0.1:<port>` as the shells do.

---

## 0.10.3 — iOS builds and runs

**No action needed.** This release makes `goleo build ios` work; nothing that worked before
changes.

0.10.2's entry above says the iOS build stops at the Swift compile. It no longer does. Verified
on a macOS runner from a freshly scaffolded project: `GoleoApp.app` builds for the Simulator,
installs, launches, and the screenshot shows the embedded UI rendered with `goleo:getOS`
answered by the Go backend, a custom `invoke()` returning, and `heartbeat` push events arriving.
So the Go backend, loopback asset serving, WKWebView, bridge invoke and event push all work.

```bash
goleo build ios --simulator     # needs no Apple Developer account
goleo build ios                 # device build; needs a signing certificate
```

Six things were wrong, each hidden behind the one before it — the gomobile simulator slice, the
Xcode project format, the xcframework name, the Swift binding names, XcodeGen's app-icon preset,
and a `BGTaskScheduler` identifier taken from the Android package name. That last one is the
only one worth checking yourself: if you set `mobile.ios.bundle_identifier` to something other
than your Android `package_name`, 0.10.2 would have crashed on launch (the identifier was not in
`Info.plist`'s permitted list, which raises an exception). It is derived from the iOS bundle id
now.

**Still no `.ipa`**, so TestFlight and App Store submission remain unwired; both need a paid
Apple Developer account. `goleo build ios --release` refuses and says so.

---

## 0.10.4 — Android builds no longer restrict which devices can install your app

**Affects:** Android apps that enable **Bluetooth** or **Geolocation**. **No action needed** —
your next build simply reaches more devices. Read on if you have already published, because the
effect is visible on your store listing.

Android derives a `<uses-feature>` declaration from certain permissions automatically, and an
**implied** one defaults to `android:required="true"`. goleo declared `camera`, `nfc` and
`bluetooth_le` explicitly as optional, but not the two its own permissions implied:

| Permission goleo declares | Implied feature | Was |
|---|---|---|
| `BLUETOOTH` / `BLUETOOTH_ADMIN` (legacy, `maxSdkVersion=30`) | `android.hardware.bluetooth` | **required** |
| `ACCESS_FINE_LOCATION` | `android.hardware.location` | **required** |

A required feature is a hard filter: Play hides the app from every device without that hardware.
For features that fall back to a browser API when unavailable, that is simply lost reach.

Both are now declared `required="false"`, so the build prints six optional features where it
printed three:

```
Hardware features (all optional): android.hardware.bluetooth, android.hardware.bluetooth_le,
android.hardware.camera, android.hardware.location, android.hardware.location.gps,
android.hardware.nfc
```

**If you have already shipped a release**, rebuild and upload again — your supported-device count
in Play Console should increase. Nothing else changes: the permission list is untouched, and
`android.hardware.faketouch` (also implied, by the platform) is left alone because every device
satisfies it.

**If you add a hardware permission yourself** through `mobile.android.extra_permissions`, declare
its feature too — goleo can only derive features for the permissions it adds itself.

This was found by uploading to Play for the first time. It is worth knowing that it *cannot* be
caught locally: implied features exist only after `aapt2` processes the manifest, and the only
symptom is a store-side distribution filter — nothing fails, and the app installs fine on any
device you happen to test.

---

## 0.10.5 — the build tells you when it is compiling a local checkout

**No action needed** unless you have `GOLEO_ROOT` in your workflow. Nothing changes about what
gets built; you just find out what is being built.

`GOLEO_ROOT` wires a local goleo checkout into a project via a `replace` directive in `go.mod` —
and nothing ever removed it again. So a single `goleo build` or `goleo dev` with the variable set
repointed the project **permanently**, and every later build silently compiled the checkout even
with the variable unset. The `require` line in `go.mod` became decorative.

A real project was found requiring `v0.9.3` while building a working tree several releases ahead.
Bumping the require would have looked like an upgrade and changed nothing.

Now, when `go.mod` replaces goleo with a **directory** and `GOLEO_ROOT` is not set, the build
prints:

```
  NOTE: go.mod replaces github.com/daforester/goleo with a local directory:
          E:/Development/goleo
  This build compiles THAT directory, so the require version in go.mod is not
  what you are shipping — and GOLEO_ROOT is not set on this run. Expected while
  developing goleo itself; otherwise drop it to build a released version:
          go mod edit -dropreplace github.com/daforester/goleo
          go mod tidy && go mod vendor
```

**If you are developing goleo itself**, set `GOLEO_ROOT` and the notice stays quiet — it only
fires when the pin is in `go.mod` but the variable is not in the environment, which is the state
that misleads. **If you pin goleo to a fork** (`=> github.com/you/goleo v1.2.3`) nothing changes:
a module replacement carries a version, a directory replacement does not, and only the latter is
flagged.

Nothing is removed for you. The replace is legitimate; it was only ever a problem for being
invisible.

---

## 0.10.7 — the mobile WebView only grants camera, mic and location to your own UI

**Affects:** Android and iOS apps whose WebView loads content from anywhere other than the
app's own loopback origin — a remote page, a CDN-hosted view, an embedded third-party frame —
**and** that uses `getUserMedia` or `navigator.geolocation` from that content.

### What changed

The native shells answer the WebView's camera, microphone and geolocation permission prompts.
They were not checking properly who was asking:

- **iOS granted all three unconditionally**, ignoring the origin entirely.
- **Android matched a string prefix** (`startsWith("http://127.0.0.1")`), which also accepts
  `http://127.0.0.1.evil.com` — an ordinary domain anyone can register.
- **Android's geolocation callback had no origin check at all.**

Neither shell restricts navigation, so those gates answered for whatever page the WebView had
reached. All three now parse the origin and require the **host** to be loopback
(`127.0.0.1`, `localhost`, `::1`), which is what goleo serves your UI from. The Android *dev*
shell additionally accepts `10.0.2.2`, the emulator's alias for your machine's loopback.

### Do I need to change anything?

Only if your UI is served from somewhere other than goleo's own loopback server. If it is,
those three APIs now fail in the WebView. There is no config switch — host your UI through
goleo (the default) or use the Go-side feature APIs, which are gated by `Policy` instead.

Everything the scaffolds produce is unaffected: the frontend is embedded in the Go binary and
served over loopback on both platforms.

---

## 0.10.7 — build flags are refused by targets that cannot honour them

**Affects:** scripts and CI jobs that pass `--ios-target`, `--android-api`, or `--ios-team` to
a `goleo build` invocation that does not use them.

### What changed

These three flags were declared globally and read by exactly one target each, so every other
target accepted them and silently did nothing — `goleo build windows --ios-target 17.0`
reported success having ignored the flag. They now fail fast:

| Flag | Accepted by |
|---|---|
| `--ios-target`, `--ios-team` | `ios` only |
| `--android-api` | `android` only |

`--ios-team` is additionally refused with `--simulator`, since a Simulator build is not signed
at all — that is what makes it work without an Apple Developer account.

### Do I need to change anything?

Only if a build command passes one of these to the wrong target. The error names the flag and
the target it applies to. This matches how `--bundle`, `--publish`, `--release`,
`--android-format`, `--version-code`, `--android-abi` and `--android-ndk` have behaved since
0.10.0 — a flag is either honoured or refused, never accepted and ignored.

---

## 0.10.7 — iOS apps expose their Documents directory to the Files app

**Affects:** every iOS app built with goleo.

### What changed

The generated `Info.plist` now sets `UIFileSharingEnabled` and
`LSSupportsOpeningDocumentsInPlace`, so the app's Documents directory appears in the iOS Files
app under the app's name.

This is required for `goleo:dialogSaveFile` to be useful. iOS has no "choose a destination,
then write to it" primitive — the document picker exports a file that already exists, while
the caller needs a path to write to — so on mobile `saveFile` asks for a *name* and returns a
path in Documents. Without these two keys the file is written somewhere the user can never
reach.

### Do I need to change anything?

Nothing to change, but be aware that anything your app writes to its Documents directory is
now user-visible and user-deletable. If that is wrong for your app, write to Application
Support instead (`goleo:fsAppDataDir`), which stays private, and remove the two keys from the
generated `Info.plist` — note that goleo regenerates `.goleo/ios/` on every build, so that
edit does not survive; open an issue if you need it configurable.
