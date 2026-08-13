# CLI commands and build targets

**Read this before changing `cli/cmd/` — adding or renaming a command, changing what a build
target produces, or touching the flags any of them accept.**

Moved out of `AGENTS.md` on 2026-08-13: it is ~1.5k tokens of `cli/` detail that every session
paid for, including sessions working only in `runtime/` or the docs. The **build-flag validation**
rules stayed in `AGENTS.md` on purpose — the CI gates they describe (`deadcode`,
`staticcheck -checks U1000`) cover `runtime/` too, and "do not park a helper for later" is an
agent directive rather than CLI trivia.

Most of the table below is derivable from `goleo --help`. What is **not** derivable, and is the
reason this file exists rather than pointing at `--help`, is the per-command behaviour: which
targets refuse rather than warn, what each one needs configured first, and which paths need an
Apple or Play account at all.

### Commands

| Command | Description |
|---------|-------------|
| goleo new <name> | Scaffold a new Goleo project (prompts for minimal vs demo; `--demo` / `--template`) |
| goleo dev | Start development mode (Go + Vite with HMR) |
| goleo dev pwa | Start PWA development mode (Vite only, no Go backend) |
| goleo build | Build for current platform |
| goleo build windows | Cross-compile for Windows amd64 |
| goleo build linux | Cross-compile for Linux amd64 |
| goleo build darwin | Cross-compile for macOS amd64 |
| goleo build android | Build an unsigned debug .apk (gomobile AAR + Gradle) |
| goleo build android --release | Build a **signed .aab** for Play (`--android-format apk` for a signed APK) |
| goleo build ios | Build a debug iOS `GoleoApp.app` for a **device** (gomobile xcframework + xcodegen + xcodebuild). Needs `mobile.ios.development_team` (or `--ios-team`); refused early without one |
| goleo build ios --simulator | Build an **unsigned Simulator** app — the only iOS path that needs no Apple Developer account |
| goleo build pwa | Build Progressive Web App (no Go backend) |
| goleo build --bundle | Also package the desktop app into a native installer (dist/bundle/) |
| goleo build --publish | Also write an ed25519-signed update manifest (needs GOLEO_UPDATE_PRIVKEY) |
| goleo emulate android | Run in dev mode on a connected Android device or emulator |
| goleo install android | Sideload the built app.apk onto a connected device + launch it |
| goleo generate types | Generate frontend/src/goleo.d.ts (typed invoke() overloads) |
| goleo generate updater-key | Generate an ed25519 keypair for signing update manifests |
| goleo generate android-key | Generate an Android signing keystore (uses the JDK goleo resolves, so keytool need not be on PATH) |
| goleo doctor android | Report whether the android build/emulate dependencies are present — discovery only, installs nothing and never prompts (so CI and editor tooling can check readiness safely) |
| goleo version | Print version |

### Build Targets

| Target | GOOS | GOARCH | Output | Dependency |
|--------|------|--------|--------|------------|
| current | auto | auto | `app.exe` on Windows, `app` elsewhere | none |
| windows | windows | amd64 | .exe | none |
| linux | linux | amd64 | binary | none |
| darwin | darwin | amd64 | binary | none |
| android | android | arm64 | `app.apk` (debug) or `app.aab` (`--release`) | gomobile + NDK |
| ios | ios | arm64 + arm64-simulator | `GoleoApp.app` (debug; the `.xcframework` is an intermediate and is deleted) | gomobile + Xcode (+ a Team ID for device builds) |
| pwa | js | wasm | dist-pwa/ | none |
