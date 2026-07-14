# 1. Installation

## Prerequisites (all platforms)

- **Go 1.26+** — the backend + CLI. <https://go.dev/dl/>
- **Node.js 18+** and npm — the frontend toolchain (Vite).

That's all you need for desktop development and cross-compiled desktop builds.
Extra tools are only needed for native installers and mobile (see below).

## Install the Goleo CLI

Two options — pick one:

**Go install (recommended):**
```bash
go install github.com/daforester/goleo/cli/goleo@latest
```
Ensure `$(go env GOPATH)/bin` is on your `PATH` so `goleo` is found.

**npm scaffold (also installs a bundled CLI):**
```bash
npm create goleo-app@latest my-app
```

Verify:
```bash
goleo version
```

## Desktop runtime libraries

Goleo renders into the OS's own webview — nothing is bundled, so the system
library must be present (it usually already is):

| OS | Needs | Notes |
|----|-------|-------|
| **Windows** | WebView2 Runtime (Edge) | Preinstalled on Windows 10/11. |
| **macOS** | WKWebView | Part of the OS. |
| **Linux** | WebKitGTK (`libwebkit2gtk-4.1` / `libwebkitgtk-6.0`) + GTK | Install via your package manager, e.g. `sudo apt install libwebkit2gtk-4.1-0`. |

Desktop builds are `CGO_ENABLED=0` and cross-compile from any host — you do **not**
need a C toolchain.

## Optional: native installers

Producing installers (`goleo build --bundle`) shells out to per-OS packagers,
detected at build time. Install only the one(s) you target:

| Target installer | Tool | Install |
|------------------|------|---------|
| Windows `.exe` (NSIS) | `makensis` | e.g. `choco install nsis` |
| macOS `.dmg` / `.app` | `hdiutil` (built in) | — |
| Linux `.deb` / `.rpm` | `nfpm` | `go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest` |

The app icon + version metadata come from `goleo.json` and need no extra tools —
see [Packaging, icons & metadata](04-packaging-icons.md).

## Optional: mobile

Building/running for Android or iOS needs the platform SDKs:

**Android** (any host):
- Android SDK **platform-tools** (`adb`) and an **NDK**.
- A JDK (for Gradle).
- Goleo auto-resolves `gomobile`; point it at your SDK/NDK via the standard
  `ANDROID_HOME` / `ANDROID_NDK_HOME` env vars, or pass `--android-ndk`.

**iOS** (macOS only):
- Xcode + command-line tools.

See [Mobile](10-mobile.md) for device dev and sideloading.

---

Next: [Project setup →](02-setup.md)
