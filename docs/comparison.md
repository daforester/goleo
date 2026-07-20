# Goleo vs Tauri v2 vs Wails v3

> How Goleo relates to the two dominant "native-webview + web UI, no bundled
> Chromium" desktop frameworks. Facts current as of **July 2026**. For Goleo's
> own architecture see [`AGENTS.md`](../AGENTS.md); for the feasibility evidence
> behind Goleo's cgo-free claims see [`SPIKES.md`](../SPIKES.md).

All three frameworks share the same foundation: a compiled backend, the OS-native
webview (**WebView2** on Windows, **WKWebView** on macOS, **WebKitGTK** on Linux),
a web frontend of your choice, and **no bundled browser runtime** — so all three
land in the same ~3–15 MB binary range, far below Electron's ~50–150 MB. The
meaningful differences are **language**, **build / cross-compile posture**, the
**frontend↔backend transport**, and **maturity**.

## One-line positioning

- **Tauri v2** — the mature, audited incumbent. Rust-mandatory core.
- **Wails v3** — Go-native, in-process, first-class multi-window/mobile — but alpha, and still cgo-bound on mac/Linux.
- **Goleo** — earliest-stage of the three, but the only one chasing a **fully cgo-free, cross-compile-from-anywhere** thesis and the only one with **PWA as a first-class build target**.

## At a glance

| Dimension | **Goleo** | **Tauri v2** | **Wails v3** |
|---|---|---|---|
| Core language | **Go** | **Rust** (mandatory) | **Go** |
| Status | Early / pre-release | **Stable** (2.0 Oct 2024, v2.11.x), ~109k★, security-audited | **Alpha** (v3.0.0-alpha2.117), ~35k★; v2 is the stable line |
| Webview | WebView2 / WKWebView / WebKitGTK | same (via WRY) | same |
| Bundled runtime | No (system webview) | No | No |
| **cgo posture** | **Windows cgo-free (shipping); mac/Linux still cgo today** — cgo-free purego path *proven in spikes, not yet integrated* | Not cgo, but **Rust toolchain + per-OS C deps** required | **Windows cgo-free; mac & Linux require cgo** (Xcode / libwebkit2gtk) |
| Cross-compilation | Windows builds+cross-compiles `CGO_ENABLED=0`; darwin cross-built from Windows *in the spike*. Full 3-OS cross-compile is the thesis, **not fully shipped** | Discouraged — msi=Windows-only, deb/appimage=Linux-only → **use per-OS CI** | mac/Linux cross-compile only **via Docker** (~800 MB image w/ Zig + macOS SDK) |
| Frontend↔backend | **Native in-process IPC (opt-in `Config.NativeIPC`) → WebSocket → HTTP** — portless for the primary window when enabled, server-backed otherwise | In-process IPC (custom-protocol JSON-RPC) — no port | In-process in-memory bindings — no port |
| Typed bindings | `goleo generate types` → `goleo.d.ts` overloads | `invoke()` + plugin typings | `wails3 generate bindings` (static analyzer, TS) |
| Multi-window | **Multi-process children by default**; opt-in in-process on Windows | In-process (TAO/WRY) | In-process, first-class in v3 |
| Mobile | gomobile `.aar` / `.xcframework`, WebView shell | **First-class, stable-ish** (parity still closing) | **New in v3, alpha** (same `main.go`) |
| Security model | **Policy Allow-list ACL** (`runtime/policy.go`) + **server hardening** (loopback bind, WS origin allow-list, per-launch token) | **Capabilities / Permissions / Scopes** (deny-by-default) + encrypted **Isolation Pattern** iframe; externally audited | Plain Go stdlib access; no capability ACL |
| Distribution | NSIS / `.app`+`.dmg` / `.deb`+`.rpm` (nfpm); Authenticode + codesign | deb / rpm / appimage / nsis / msi / app / dmg; signing | NSIS / MSIX, `.app` (DMG is DIY), deb / rpm / appimage |
| Auto-updater | ed25519-signed `manifest.json` | official updater plugin (separate signing key) | built-in, **binary delta patches** |
| PWA target | **Yes — `goleo build/dev pwa` (js/wasm)** | No | No |
| Native feature APIs | 13 host features + 9 core, provider pattern | Large official + community plugin ecosystem | Dialogs / menu / tray / clipboard / notifications / shortcuts built in |

## What genuinely sets Goleo apart

1. **The cgo-free thesis.** This is the real differentiator versus *both*. Wails
   needs cgo on mac + Linux; Tauri needs the Rust toolchain + system C deps.
   Goleo's spikes proved `purego`+`dlopen` (Linux) and `purego`+objc WKWebView
   (macOS, on real Apple Silicon) work under `CGO_ENABLED=0`, plus darwin
   cross-built from Windows. **Delivered:** all three desktops now ship on the
   cgo-free `glaze` binding (WKWebView / WebKitGTK / WebView2 via `purego`), so the
   whole runtime builds `CGO_ENABLED=0` and cross-compiles from one machine. The
   legacy cgo `webview_go` backend remains only as an opt-in fallback (macOS/Linux,
   one release then removed).

2. **PWA as a build target.** Neither competitor ships your app as an installable
   PWA (js/wasm). For Goleo this falls out of the WebSocket/HTTP bridge plus the
   bridge's graceful browser-API degradation.

3. **Go without Wails' cgo tax.** If the purego path lands, Goleo becomes "Wails
   ergonomics, Tauri-style cross-compile freedom, in Go" — a coherent, defensible
   pitch that neither competitor currently occupies.

## Where Goleo is behind (clear-eyed)

- **Maturity & ecosystem.** Tauri (~109k★, audited, large plugin workspace) and
  even Wails v2 (stable, ~35k★) are years and communities ahead. Goleo is
  single-author and pre-release.
- **The server model is double-edged** (now partly addressed). Tauri and Wails
  have *no localhost port* — IPC is in-process, so there is no network attack
  surface at all. Goleo runs a real WebSocket/HTTP server, which is why it needs
  its loopback-bind + origin-allow-list + per-launch-token hardening. **`Config.NativeIPC`
  (opt-in) now gives the primary window a portless in-process channel** (webview
  `Bind`/`Eval`, `runtime/nativeipc.go`), closing that surface for it — but the
  HTTP server still runs to serve embedded assets and to back child-process
  windows / browser / PWA / mobile. Full parity (custom-scheme asset serving to
  drop the asset server too, and native IPC for additional windows) is a follow-up.
- **Multi-window default is multi-process** (extra RAM/startup per window) where
  both competitors are in-process. Goleo's in-process path closes this on Windows
  only.
- **Security depth.** Tauri's audited capabilities+scopes+encrypted-isolation is
  more battle-tested than Goleo's Allow-list `Policy` — conceptually parallel,
  very different maturity.
- **Mobile is unproven** across all three's newer paths, but Tauri's is furthest
  along and shipping production apps.

## When to pick which

- **Tauri v2** — maximum maturity, security-audit pedigree, and ecosystem, if you
  accept Rust.
- **Wails v3** — Go ergonomics with first-class in-process multi-window and mobile
  *today*, if you accept cgo on mac/Linux and alpha risk.
- **Goleo** — the intersection neither fills: **Go + genuinely cgo-free +
  cross-compile-from-one-machine + PWA output.** Landing the purego mac/Linux
  runtime is what turns that from "interesting spike" into "the reason you'd pick
  it."

## Sources

- Tauri: [2.0 stable](https://v2.tauri.app/blog/tauri-20/), [process model](https://v2.tauri.app/concept/process-model/), [IPC](https://v2.tauri.app/concept/inter-process-communication/), [security/capabilities](https://v2.tauri.app/security/), [prerequisites](https://v2.tauri.app/start/prerequisites/), [Windows installer](https://v2.tauri.app/distribute/windows-installer/), [updater](https://v2.tauri.app/plugin/updater/).
- Wails: [v3 status](https://v3.wails.io/status/), [what's new](https://v3.wails.io/whats-new/), [architecture](https://v3.wails.io/concepts/architecture/), [cross-platform build](https://v3.wails.io/guides/build/cross-platform/), [mobile](https://v3.wails.io/guides/mobile/), [GitHub releases](https://github.com/wailsapp/wails/releases).
- Goleo: [`AGENTS.md`](../AGENTS.md), [`SPIKES.md`](../SPIKES.md), [`docs/roadmap.md`](roadmap.md).

> Star counts and binary-size figures for Tauri/Wails are from vendor/community
> sources, accurate as ballpark rather than to the kilobyte.
