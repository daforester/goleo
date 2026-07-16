# 4. Packaging, icons & metadata

All packaging is driven by the `bundle` section of `goleo.json` — one place to set
your app icon, identity, and metadata for both the **executable** and the **native
installers**.

## The `bundle` config

```jsonc
{
  "app_name": "My App",
  "version": "1.2.3",
  "bundle": {
    "identifier": "com.example.my-app",   // reverse-DNS app id (macOS bundle id, etc.)
    "publisher":  "Example Ltd",           // company / maintainer
    "description":"Does something useful", // one-line description
    "copyright":  "© 2026 Example Ltd",
    "category":   "Utility",               // freedesktop / store category
    "homepage":   "https://example.com",

    "icon":       "assets/icon.png",       // single source icon (recommended)
    "icon_ico":   "",                      // optional explicit Windows .ico override
    "icon_icns":  "",                      // optional explicit macOS .icns override
    "icon_png":   ""                       // optional explicit Linux .png override
  }
}
```

### The app icon

Provide **one** `icon` — a square PNG, ideally **1024×1024** (it's downscaled, not
upscaled) — and Goleo generates every per-platform artifact from it, in pure Go, at
build time (no ImageMagick / iconutil / external tooling). For full control, set the
explicit `icon_ico` / `icon_icns` / `icon_png` paths, which override `icon`.

| Where the icon shows up | Source | Generated artifact |
|-------------------------|--------|--------------------|
| **Windows `.exe`** (Explorer, taskbar) | `icon_ico`, else `icon` | Multi-size `.ico` (16→256) embedded in the exe |
| **Windows installer** (NSIS) | same | (uses the embedded exe icon) + version metadata |
| **macOS `.app` / `.dmg`** | `icon_icns`, else `icon` | `icon.icns` (32→1024, all Retina scales) |
| **Linux `.deb`/`.rpm`** | `icon_png`, else `icon` | 256×256 hicolor PNG + a `.desktop` launcher entry |
| **Android** launcher | `icon` | `mipmap-*/ic_launcher.png` + round variants (all densities) |
| **iOS** app icon | `icon` | `AppIcon.appiconset` (1024 universal) wired via xcodegen |

Every artifact is derived automatically — you don't run any icon step. If no `icon`
(and no explicit override) is set, each platform keeps its default icon.

> Tip: put your source icon at `assets/icon.png` (the scaffold points there).

### Executable metadata (Windows)

`goleo build windows` embeds a version resource into `app.exe` so right-click →
**Properties → Details** shows real values, sourced from `goleo.json`:

| Details field | From |
|---------------|------|
| File description | `bundle.description` (falls back to `app_name`) |
| Product name | `app_name` |
| File / product version | `version` |
| Company | `bundle.publisher` |
| Copyright | `bundle.copyright` |

This is automatic on every Windows build — no extra flags. (The generated
`.syso` resource is created in the build dir and cleaned up afterward; it's
gitignored.)

## Native installers

```bash
npm run goleo:bundle            # current OS
npm run goleo:bundle-windows    # NSIS .exe installer
npm run goleo:bundle-linux      # .deb + .rpm (via nfpm)
npm run goleo:bundle-darwin     # .app + .dmg
```

Or: `goleo build <target> --bundle`. Output lands in `dist/bundle/`
(default name `<app>-<version>-setup.exe` / `.dmg` / `.deb`). Pass `-o NAME` to
name the installer — e.g. `goleo build windows --bundle -o myapp` →
`dist/bundle/myapp-setup.exe` (the `-setup` suffix keeps it distinct from the
`myapp.exe` binary).

Each installer reads the same `bundle` metadata:

- **Windows (NSIS)**: app name, install dir, Start-menu shortcut, uninstaller, and
  version metadata (`VIProductVersion`, product/company/copyright).
- **macOS**: a `.app` with your `CFBundleIdentifier`, version, and a generated
  `icon.icns`, wrapped in a `.dmg`.
- **Linux**: `.deb`/`.rpm` with name, version, maintainer (`publisher`),
  `description`, `homepage`, `category`, a generated hicolor icon, and a
  `.desktop` launcher entry (so it appears in the applications menu).

> Installers use the per-OS packager (`makensis`, `hdiutil`, `nfpm`). On Windows,
> `goleo build --bundle` **auto-installs NSIS** if `makensis` is missing (via
> `winget`, `choco`, or `scoop`, and it finds NSIS's standard install dir even when
> it's not on `PATH`). Set `GOLEO_NO_INSTALL=1` to disable auto-install and get a
> plain error instead. See [Installation](01-installation.md).

## Mobile identity

Set the Android package name and iOS bundle id under `mobile` in `goleo.json`:

```jsonc
"mobile": {
  "android": { "min_sdk": 24, "package_name": "com.example.myapp" },
  "ios":     { "deployment_target": "14.0", "bundle_identifier": "com.example.myapp" }
}
```

The **launcher icon** on both mobile platforms is generated from the single
`bundle.icon` (see the table above) — Android gets density-bucketed
`res/mipmap-*/ic_launcher.png` (square) and `ic_launcher_round.png` (circular),
referenced from the merged `AndroidManifest.xml`; iOS gets an
`AppIcon.appiconset` wired into the Xcode project. No per-platform icon assets to
maintain by hand.

## Signing (optional)

Code signing is env-driven and applied during `--bundle`:

- **Windows**: Authenticode via your signing cert (set the documented signing env
  vars before `goleo build windows --bundle`).
- **macOS**: `codesign` + `notarytool` when the signing identity env vars are set.

See `cli/cmd/signing.go` for the exact env variables your CI should provide.

---

Next: [Deploying & updating →](05-deploy.md)
