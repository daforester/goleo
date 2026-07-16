# 2. Project setup

## Scaffold a project

```bash
goleo new my-app
cd my-app
cd frontend && npm install && cd ..
```

`goleo new` asks which starter to use:
- **minimal** — a clean starter app (default)
- **demo** — a full showcase of every host feature (camera, geolocation,
  clipboard, dialogs, notifications, battery, sensors, BLE, NFC, …)

Skip the prompt with a flag: `goleo new my-app --demo` (or
`goleo new my-app --template minimal`).

(No global install? `npx @goleo/cli new my-app` does the same.)

`goleo new` also runs `go mod vendor`, so the project ships with a committed
`vendor/` containing all Go dependencies — including the pinned `glaze` webview
fork. Your project then **builds offline** and is insulated from upstream
changes (matching how goleo itself vendors). Commit `vendor/`.

## Project structure

```
my-app/
├── goleo.json              # Project config: app name, version, bundle metadata, mobile
├── go.mod                  # Go module (pins the glaze webview fork via a replace)
├── package.json            # Root: the goleo:* npm scripts
├── backend/
│   ├── app/app.go          # ★ The one file you edit: startup, config, commands
│   ├── commands/commands.go# Your backend commands (RPC handlers)
│   ├── init.js             # Optional JS startup (window creation) — advanced
│   ├── main.go             # GENERATED (do not edit; gitignored)
│   └── gomobile/           # GENERATED mobile entry points (gitignored)
└── frontend/
    ├── index.html
    ├── vite.config.ts      # Vite dev server + API proxy
    └── src/
        ├── main.ts         # Frontend entry: initializes @goleo/bridge
        └── App.vue         # Your root component
```

- **You edit `backend/app/app.go`** (all app-specific Go: window config, commands,
  feature wiring) and everything under `frontend/src/`.
- `backend/main.go` and `backend/gomobile/*` are **generated fresh** on every
  `goleo dev`/`build`/`emulate` — never edit them (they're gitignored). They just
  call `app.New(...)`.

## Run in development

```bash
npm run goleo:dev
```

This starts:
- **Vite** on port 5173 with hot module replacement (instant frontend reloads).
- The **Go backend** on port 9842, with Vite proxying `/api` + `/ws` to it.
- A native window pointed at the dev server.

Edit `frontend/src/**` → the UI updates instantly. Edit Go → stop and re-run
`goleo:dev` (backend live-reload is on the roadmap).

Other dev modes:
```bash
npm run goleo:dev-pwa        # frontend only, in a browser (no Go backend)
npm run goleo:dev-android    # on a connected Android device or emulator
```

## `goleo.json`

The project manifest. Key sections:

```jsonc
{
  "version": "0.1.0",
  "app_name": "My App",
  "frontend": { "directory": "frontend", "build_command": "npm run build", "dist_dir": "dist" },
  "bundle": {                       // installer + executable metadata (see page 4)
    "identifier": "com.example.my-app",
    "publisher": "Example Ltd",
    "description": "Does something useful",
    "copyright": "© 2026 Example Ltd",
    "icon": "assets/icon.png"       // single source icon; per-platform overrides also allowed
  },
  "mobile": {
    "android": { "min_sdk": 24, "package_name": "com.example.myapp" },
    "ios":     { "deployment_target": "14.0", "bundle_identifier": "com.example.myapp" }
  }
}
```

## The npm scripts

Everything routes through `goleo` under the hood; the scaffold gives you:

| Script | What it does |
|--------|--------------|
| `goleo:dev` / `goleo:dev-pwa` / `goleo:dev-android` | Development (desktop / PWA / mobile device) |
| `goleo:build[-os]` | Standalone binary (this OS or a named target) |
| `goleo:build-android` | Installable `app.apk` |
| `goleo:build-pwa` | Progressive Web App to `dist-pwa/` |
| `goleo:bundle[-os]` | Native installer (NSIS / .dmg / .deb+.rpm) |
| `goleo:publish` | Signed auto-update manifest |
| `goleo:sideload-android` | Build the APK **and** install it on a connected device |
| `goleo:types` | Generate typed `invoke()` declarations (`frontend/src/goleo.d.ts`) |

---

Next: [Building →](03-building.md)
