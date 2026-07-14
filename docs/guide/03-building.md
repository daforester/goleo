# 3. Building

Goleo produces three kinds of output:

- **Standalone binaries** — a single self-contained executable (frontend embedded).
- **Native installers** — see [Packaging, icons & metadata](04-packaging-icons.md).
- **Mobile packages** — an Android `.apk` or iOS `.xcframework`.

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
npm run goleo:build-ios        # -> .xcframework (macOS + Xcode)
```

- **Android**: builds a gomobile AAR, generates an Android project, and compiles
  an installable `app.apk` with Gradle. Needs the Android SDK + NDK.
- **iOS**: builds an `.xcframework` to integrate into an Xcode app. macOS only.

To run on a real device during development, or to sideload the APK, see
[Mobile](10-mobile.md).

## Choosing the webview backend (advanced)

The default desktop backend is the cgo-free **glaze** binding on all three OSes.
Two opt-in fallbacks exist for one release (then removed) if you hit a backend bug:

```bash
GOLEO_CGO_WEBVIEW=1 goleo build linux    # macOS/Linux: legacy cgo webview_go (needs CGO + toolchain)
goleo build windows -- -tags goleo_webview2   # Windows: legacy go-webview2
```

You should not normally need these.

## What the version string is

Builds stamp the binary with `goleo.json`'s `version` (via `-ldflags -X
main.Version=...`), and — on Windows — embed it in the executable's version info
(see the next page).

---

Next: [Packaging, icons & metadata →](04-packaging-icons.md)
