# External binaries goleo shells out to

**Read this before adding a feature that calls an external program, before changing what a
`runtime/<feature>/` package invokes, and before assuming a feature "works on Linux".**

goleo is `CGO_ENABLED=0` everywhere (see [`webview.md`](webview.md)). That invariant buys
single-machine cross-compilation for every desktop target, and the bill comes due here: a Go
program with no cgo cannot call CoreLocation, `IAudioClient`, or GTK directly. So a number of
host features reach the OS by **launching another process** instead.

That is a deliberate trade, not an accident. But it has consequences that are easy to forget:

- **A shelled-out feature can be absent at runtime on a machine that compiled it fine.**
  `xclip` and `zenity` are not installed by default on many Linux images, and there is no
  build-time signal.
- **Every call is a process launch.** Fine for a clipboard write on a button press; not fine
  in a loop or on a timer.
- **Cost is invisible in tests.** CI runs headless Linux, where several of these are absent,
  so the tests that cover them skip rather than fail — deliberately, but it means green CI is
  not evidence the path works.
- **The webview is often a better caller of the same OS API.** That is exactly why geolocation
  stopped having a Go implementation — see the table note below.

Two separate inventories follow, because the blast radius is completely different:
**runtime** binaries are a dependency of *the app you ship to users*, while **CLI** binaries
are a dependency of *your build machine*.

---

## 1. Runtime — dependencies of the shipped app

These run on the end user's machine. A missing binary here is a feature that fails for a user.

| Feature | Windows | macOS | Linux | Missing-binary behavior |
|---|---|---|---|---|
| **Clipboard** | native Win32 API | `pbcopy` / `pbpaste` | `xclip` | error to the caller; bridge falls back to `navigator.clipboard` |
| **Dialogs** | `powershell` | `osascript` | `zenity` | error; bridge falls back to `<input type="file">` where one exists |
| **Notifications** | native Win32 (toast) | `osascript` | `notify-send` (`LookPath`-checked) | Linux reports unsupported rather than failing mid-call |
| **Battery** | native Win32 API | `pmset -g batt` | `/sys/class/power_supply` (file read, no process) | error; bridge falls back to `navigator.getBattery()` |
| **WakeLock** | native `SetThreadExecutionState` | `caffeinate` | `systemd-inhibit` (`LookPath`-checked) | error; bridge falls back to `navigator.wakeLock` |
| **Share** | `rundll32 url.dll,FileProtocolHandler` | `open` | `xdg-open` | error; bridge falls back to the Web Share API |
| **openURL** (`goleo:openURL`) | OS handler | OS handler | OS handler | error. **Scheme-allow-listed first** — see the security note below |
| **Deep link registration** | `HKCU\Software\Classes` (registry) | `.app` Info.plist at bundle time | `xdg-mime`, `update-desktop-database` | best-effort; both Linux calls ignore failure by design |
| **Updater** | relaunches **itself** | itself | itself | not third-party |
| **Extra windows** | relaunches **itself** as a child process | itself | itself | not third-party |

### Notably NOT in this table

- **Geolocation.** It has no Go implementation at all any more — it is a pure web feature on
  every platform (`navigator.geolocation`). It used to be here twice: a **PowerShell 5.1
  subprocess** driving the WinRT `Geolocator` on Windows, and a shell-out to
  **`CoreLocationCLI`** on macOS *if the user happened to have `brew install corelocationcli`*.
  One of six platforms had a working native path and it launched a process to fetch a
  coordinate; the other five already used the browser. The webview reaches the same OS service
  with the real permission UI and no subprocess. `runtime.RegisterGeolocation` still exists and
  must still be called — it declares `ACCESS_FINE_LOCATION` and
  `NSLocationWhenInUseUsageDescription`, without which the WebView's own request is denied.
- **Camera and microphone.** Also pure web on every platform, by design: the live preview is
  `getUserMedia` → `<video>` and recording is `MediaRecorder`. A native provider cannot supply
  a live preview into the WebView's DOM without pumping frames over the bridge. (Linux has a
  native V4L2 still-photo path in Go — no subprocess.)
- **Mobile.** Android and iOS shell out to nothing. Every mobile feature is either a native
  `Provider` in the Java/Swift shell or a web API, because a mobile app cannot spawn helper
  binaries anyway. **This is why "works on desktop" says nothing about mobile, and vice
  versa.**

### Security note, because two of these take user-controlled input

`goleo:openURL` and Share both hand a string to an OS handler. `openURL` is **scheme
allow-listed** in `runtime/platform.go` (`http`/`https`/`mailto`/`tel` + the app's own
`Config.URLScheme`); `file://`, UNC paths and bare filesystem paths are refused, because the
OS handler would otherwise happily open an executable. Anything new here that reaches
`exec.Command` with app- or page-supplied data needs the same treatment — see the Security
section of `AGENTS.md`.

Where a payload could contain shell metacharacters, the pattern is to pass it on **stdin**
rather than in the command line: `clipboard_darwin.go` and `clipboard_linux.go` both note that
`pbcopy`/`xclip` take the text on stdin "so text is never exposed to shell parsing". Follow
that, and never build a command string.

### Testing convention

These packages guard with `LookPath` and **skip** rather than fail when the tool is absent —
CI is headless Linux with no `xclip`, `zenity` or notification daemon. That is intended, but it
means a green test run does not prove any of this works. `SPIKES.md` has an entry
(2026-08-05) on a skip-guard that hid a real defect; the rule that came out of it is that a
skip must be provably a *tool-absent* skip, not a catch-all.

---

## 2. CLI — dependencies of the build machine

These are `goleo` build-time tools. A missing one fails a build, never a shipped app. Most are
resolved and reported by `goleo doctor android`; the rest fail with an install hint.

| Purpose | Tools | Notes |
|---|---|---|
| **Go / mobile bind** | `go`, `gomobile`, `gobind` | `gomobile`/`gobind` are installed as tools and resolved per build (`-mod=mod`); x/mobile is deliberately not vendorable |
| **Android build** | `gradlew` (vendored wrapper JAR), `java`, `adb`, `keytool` | goleo resolves its own JDK, so `keytool` and `java` need not be on `PATH`. The wrapper JAR is **vendored and embedded**, not downloaded |
| **Android SDK mgmt** | `sdkmanager`, `avdmanager`, `emulator` | `.bat` launchers on Windows cannot be run via `CreateProcess`, hence the `cmd /s /c` line built by `windowsSdkToolCmdLine` |
| **iOS build** | `xcodegen`, `xcodebuild`, `xcrun`, `codesign`, `notarytool`, `stapler` | macOS only. XcodeGen silently overrides project settings — see `store-submission.md` |
| **Frontend** | `npm`, `npx` (Vite) | |
| **Windows installer** | `makensis` (NSIS) | `ensureMakensis` **auto-installs NSIS** if absent |
| **Windows MSIX** | `makeappx.exe`, `signtool` | Both from the Windows SDK; `findMakeAppx` searches SDK roots since neither is on `PATH` by default |
| **Windows signing (release CI)** | `osslsigncode` | Deliberately not `signtool`: the release job cross-compiles every target from one Linux runner and `signtool` is Windows-only. `cli/cmd/signing.go`'s `signWindows` is the separate signtool path for apps built *with* goleo on Windows |
| **macOS installer** | `hdiutil`, `codesign`, `xcrun` | macOS only |
| **Linux packages** | `nfpm` | `go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest` |
| **Misc** | `taskkill` (Windows), `cmd` | process cleanup / `.bat` launching |

---

## Adding a feature that shells out

1. **Check whether the webview already does it better.** If the capability exists as a web
   API, the browser path reaches the same OS service with a real permission prompt and no
   process launch. Geolocation is the worked example of getting this wrong first.
2. **`LookPath` before use**, and return `errors.ErrUnsupported` when absent — never a generic
   error. Callers branch on it to pick a browser fallback (`AGENTS.md`, host features).
3. **Never interpolate untrusted data into a command line.** stdin, or an argv slice.
4. **Add the tool to the table above**, and to `goleo doctor` if it is a build dependency.
5. **Write the fallback in the TS wrapper**, and make it degrade — but never invent a
   plausible value on failure. A failed `showMessage` that reads as "the user clicked OK" is a
   destructive-action hazard; that bug has shipped here before.
