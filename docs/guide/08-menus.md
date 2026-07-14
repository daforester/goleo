# 8. Native menus

Goleo installs a real native menu bar on all three desktops — `NSMenu` (macOS),
a user32 `HMENU` (Windows), and GTK menus (Linux GTK3 & GTK4) — from one
declarative tree. Menus are unsupported on mobile/PWA (`MenuSupported()` reports
false there; `SetMenu` returns `ErrUnsupported`).

## Declaring the menu (Go)

Set `Config.Menu`, or call `app.SetMenu([]MenuItem)` at runtime:

```go
goleo.Config{
    // ...
    Menu: []goleo.MenuItem{
        {Label: "File", Submenu: []goleo.MenuItem{
            {Label: "New",  Accelerator: "cmd+n", OnClick: func() { /* ... */ }},
            {Label: "Open", Accelerator: "cmd+o", OnClick: func() { openFile() }},
            {Separator: true},
            {Label: "Quit", Role: goleo.RoleQuit, Accelerator: "cmd+q"},
        }},
        {Label: "Edit", Submenu: []goleo.MenuItem{
            {Label: "Undo",  Role: goleo.RoleUndo,  Accelerator: "cmd+z"},
            {Label: "Redo",  Role: goleo.RoleRedo},
            {Separator: true},
            {Label: "Cut",   Role: goleo.RoleCut},
            {Label: "Copy",  Role: goleo.RoleCopy},
            {Label: "Paste", Role: goleo.RolePaste},
            {Label: "Select All", Role: goleo.RoleSelectAll},
        }},
    },
}
```

### `MenuItem` fields

| Field | Meaning |
|-------|---------|
| `Label` | Display text |
| `Role` | A built-in behavior (below) — wires the standard action for free |
| `Accelerator` | Keyboard shortcut, e.g. `"cmd+q"`, `"ctrl+shift+p"` (`cmd` maps to Ctrl on Win/Linux) |
| `OnClick` | Go callback for a custom item |
| `Submenu` | Nested items |
| `Separator` | A divider (set `true`, ignore other fields) |

### Roles

`RoleQuit`, `RoleCopy`, `RolePaste`, `RoleCut`, `RoleSelectAll`, `RoleUndo`,
`RoleRedo`, `RoleMinimize`, `RoleClose`. Roles do the right platform thing — on
macOS they route up the responder chain so Cmd+C/V/X/A/Z work inside the webview;
on Windows/Linux they use the webview's edit commands.

### The standard menu

If `Config.Menu` is empty, Goleo installs `StandardMenu(appName)` — a sensible
App + Edit menu — so webview keyboard shortcuts work out of the box. Call
`goleo.StandardMenu("My App")` to start from it and extend.

## Driving the menu from the frontend

You can define the menu from JS and react to clicks via `@goleo/bridge`:

```ts
import { setMenu, onMenu } from '@goleo/bridge'

await setMenu([
  { label: 'File', submenu: [
    { id: 'new',  label: 'New',  accelerator: 'cmd+n' },
    { id: 'open', label: 'Open', accelerator: 'cmd+o' },
    { separator: true },
    { role: 'quit', label: 'Quit' },
  ]},
])

onMenu('new',  () => createDoc())
onMenu('open', () => openDoc())
```

Leaf items with an `id` emit a `menu:<id>` bridge event when clicked; `onMenu(id,
cb)` subscribes to it. (Under the hood this is the `goleo:setMenu` command.)

## Accelerator support by platform

| Platform | Accelerators |
|----------|--------------|
| macOS | Full |
| Linux GTK3 | Functional (`GtkAccelGroup`) |
| Windows | Edit shortcuts handled by the webview; custom-key accel tables are best-effort |
| Linux GTK4 | Best-effort |

For guaranteed shortcuts across platforms, you can also handle key events in the
frontend and call your commands directly.

---

Next: [System tray →](09-systray.md)
