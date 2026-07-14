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

Provide **one** `icon` (a square PNG — 256×256 or larger) and Goleo derives the
per-platform variants it needs. For full control, set the explicit
`icon_ico` / `icon_icns` / `icon_png` paths (these override `icon`).

| Where the icon shows up | Source | Notes |
|-------------------------|--------|-------|
| **Windows `.exe`** (Explorer, taskbar) | `icon_ico`, else generated from `icon` | Embedded into the executable at build time. |
| **Windows installer** (NSIS) | same | Plus version metadata (below). |
| **macOS `.app` / `.dmg`** | `icon_icns` | Set `icon_icns` for macOS bundles. |
| **Linux `.deb` / desktop entry** | `icon_png`, else `icon` | PNG is used directly. |

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

Or: `goleo build <target> --bundle`. Output lands in `dist/bundle/`.

Each installer reads the same `bundle` metadata:

- **Windows (NSIS)**: app name, install dir, Start-menu shortcut, uninstaller, and
  version metadata (`VIProductVersion`, product/company/copyright).
- **macOS**: a `.app` with your `CFBundleIdentifier`, version, and `icon.icns`,
  wrapped in a `.dmg`.
- **Linux**: `.deb`/`.rpm` with name, version, maintainer (`publisher`),
  `description`, `homepage`, and `category`.

> Installers require the per-OS packager (`makensis`, `hdiutil`, `nfpm`) — see
> [Installation](01-installation.md). A missing packager produces a clear error.

## Mobile identity

Set the Android package name and iOS bundle id under `mobile` in `goleo.json`:

```jsonc
"mobile": {
  "android": { "min_sdk": 24, "package_name": "com.example.myapp" },
  "ios":     { "deployment_target": "14.0", "bundle_identifier": "com.example.myapp" }
}
```

Mobile launcher icons follow the platform project conventions (Android
`res/mipmap`, iOS asset catalog).

## Signing (optional)

Code signing is env-driven and applied during `--bundle`:

- **Windows**: Authenticode via your signing cert (set the documented signing env
  vars before `goleo build windows --bundle`).
- **macOS**: `codesign` + `notarytool` when the signing identity env vars are set.

See `cli/cmd/signing.go` for the exact env variables your CI should provide.

---

Next: [Deploying & updating →](05-deploy.md)
