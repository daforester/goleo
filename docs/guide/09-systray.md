# 9. System tray

Goleo can put an icon in the system tray / menu bar with a click handler and a
menu — cgo-free on all three desktops (Windows/Linux via `gogpu/systray`, macOS via
a purego `NSStatusItem` backend). Trays are desktop-only.

## Adding a tray

Set `Config.Tray`:

```go
goleo.Config{
    // ...
    Tray: &goleo.TrayConfig{
        Tooltip: "My App",
        Icon:    trayIconPNG,          // []byte of a PNG (embed it)
        Items: []goleo.TrayItem{
            {Label: "Open",     OnClick: func() { app.OpenWindow(goleo.WindowOptions{}) }},
            {Label: "Settings", OnClick: func() { showSettings() }},
            {Separator: true},
            {Label: "Quit",     OnClick: func() { app.Quit() }},
        },
    },
}
```

(Field names follow `TrayConfig` / `TrayItem` in the runtime — an icon, a tooltip,
and a list of labelled items with `OnClick`, plus separators.)

## Tray + windows: the "hidden master" model

A native webview and the tray both want the main thread. So the common pattern for
a tray app is a **background controller** whose windows are opened on demand:

```go
goleo.Config{
    Background: true,                 // no auto primary window; main thread runs the tray
    Tray:       &goleo.TrayConfig{ /* ... */ },
    OnReady: func(ctx context.Context) {
        // window manager is up here — safe to OpenWindow if you want one at start
    },
}
```

With `Background: true`:
- No window opens automatically; the app lives in the tray until you open one
  (`app.OpenWindow`) or the user picks a tray item.
- The main thread runs the tray loop; `app.Quit()` tears everything down.

This is how you build menu-bar utilities, background sync agents, etc. Combine with
[autostart](../../AGENTS.md) (`goleo:autostart{Enable,Disable,IsEnabled}`) to launch
on login.

## Capability checks

Tray support is reported by `TraySupported()` (true on desktop, false on
mobile/PWA), and via the `goleo:capabilities` bridge query — guard tray-dependent
UI accordingly.

## Notes

- Keep the tray icon small (a PNG; platforms downscale to the tray size).
- On macOS the tray uses a menu-bar `NSStatusItem` with accessory activation (no
  dock icon needed for a pure background app).
- A tray app is still a normal Goleo app — your commands, events, menus, and host
  features all work in any windows it opens.

---

Next: [Mobile →](10-mobile.md)
