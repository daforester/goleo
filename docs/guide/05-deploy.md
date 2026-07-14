# 5. Deploying & updating

## Desktop distribution

You have two shapes to ship:

- **Standalone binary** (`goleo:build`) — hand users a single executable. Simplest;
  no install step. Good for internal tools and portable apps.
- **Native installer** (`goleo:bundle`) — an `.exe` (NSIS), `.dmg`, or `.deb`/`.rpm`
  that installs the app, a Start-menu/Applications entry, and an uninstaller. Best
  for consumer distribution. Output in `dist/bundle/`.

Sign your artifacts for a smooth install experience (see
[Packaging](04-packaging-icons.md#signing-optional)).

## Auto-updates

Goleo has a built-in signed updater.

**One-time: generate a signing key.**
```bash
goleo generate updater-key      # prints/records an ed25519 keypair
```
Keep the private key secret (CI secret `GOLEO_UPDATE_PRIVKEY`); ship the public
key in your app.

**Wire the updater in your app** (`backend/app/app.go`):
```go
import "github.com/daforester/goleo/runtime/updater"

updater.RegisterUpdater(app.Bridge(), updater.UpdaterConfig{
    ManifestURL:    "https://downloads.example.com/manifest.json",
    PublicKey:      "<your ed25519 public key>",
    CurrentVersion: "1.2.3",
})
```
The frontend can then call `goleo:updaterCheck` / `goleo:updaterApply` (both
verify the signed manifest before applying).

**Publish a release** — stages the artifact, hashes it, and merges a signed
`Release` into `manifest.json`:
```bash
GOLEO_UPDATE_PRIVKEY=... npm run goleo:publish      # goleo build --publish
```
Set `bundle.update_url_base` (and optional `release_notes`) in `goleo.json`, then
host the built artifact + `manifest.json` at that URL.

## Mobile

- **Android**: `goleo build android` produces an installable `app.apk`.
  - Sideload it to a connected device: `npm run goleo:sideload-android` (builds
    then `adb install`s + launches), or `goleo install android`.
  - For the Play Store, sign a release APK/AAB with your keystore per Android's
    standard process.
- **iOS**: `goleo build ios` produces an `.xcframework`; distribute through Xcode /
  TestFlight / the App Store.

See [Mobile](10-mobile.md) for device workflows.

## PWA

`goleo build pwa` emits a static site (`dist-pwa/`) — deploy it to any static host
(Netlify, GitHub Pages, S3, nginx…). No server component.

## A typical release checklist

1. Bump `version` in `goleo.json`.
2. `goleo:bundle-windows` / `-linux` / `-darwin` (on each OS, or CI matrix) with
   signing env set.
3. `goleo:publish` to update the signed manifest.
4. Upload installers + `manifest.json` to your download host.
5. Tag the release in git.

---

Next: [Wiring up your app →](06-wiring-apps.md)
