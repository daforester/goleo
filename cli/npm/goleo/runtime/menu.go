package runtime

import (
	"errors"
	"fmt"
)

// MenuRole is a standard menu action wired to the platform's native handler
// (e.g. the responder chain on macOS), so the item works without a Go callback.
// Roles are what make Cmd+C/V/X/A/Z etc. work in the webview on macOS.
type MenuRole string

const (
	RoleNone      MenuRole = ""
	RoleQuit      MenuRole = "quit"
	RoleUndo      MenuRole = "undo"
	RoleRedo      MenuRole = "redo"
	RoleCut       MenuRole = "cut"
	RoleCopy      MenuRole = "copy"
	RolePaste     MenuRole = "paste"
	RoleSelectAll MenuRole = "selectAll"
	RoleMinimize  MenuRole = "minimize"
	RoleClose     MenuRole = "close"
)

// MenuItem is one entry in a native menu. A top-level MenuItem (with a Submenu)
// is a menu in the menu bar; nested items are its entries. Exactly one of Role /
// OnClick / Submenu / Separator is meaningful per item (Role wins over OnClick).
type MenuItem struct {
	Label       string
	Role        MenuRole
	Accelerator string // e.g. "cmd+q", "cmd+shift+z" (cmd/ctrl/alt/shift + key)
	OnClick     func()
	Submenu     []MenuItem
	Separator   bool
}

// SetMenu installs the application menu bar. Native on macOS; returns an
// errors.ErrUnsupported-wrapped error on Windows/Linux/mobile (no native menu
// bar yet — use an in-page HTML menu there). Safe to call after Run has started
// or from Config.Menu at startup.
func (a *App) SetMenu(menu []MenuItem) error {
	if !MenuSupported() {
		return fmt.Errorf("goleo: native menu: %w", errors.ErrUnsupported)
	}
	return a.setNativeMenu(menu)
}

// MenuSupported reports whether this platform has a native menu-bar backend
// (macOS today). Query it (or goleo:capabilities) before offering menu UI.
func MenuSupported() bool { return platformMenu }

// StandardMenu returns a conventional macOS menu bar — an App menu (Quit) and an
// Edit menu (undo/redo/cut/copy/paste/select-all) — so webview keyboard
// shortcuts work. Installed automatically on macOS when Config.Menu is empty;
// also a handy base to extend for custom menus.
func StandardMenu(appName string) []MenuItem {
	if appName == "" {
		appName = "App"
	}
	return []MenuItem{
		{Label: appName, Submenu: []MenuItem{
			{Label: "Quit " + appName, Role: RoleQuit, Accelerator: "cmd+q"},
		}},
		{Label: "Edit", Submenu: []MenuItem{
			{Label: "Undo", Role: RoleUndo, Accelerator: "cmd+z"},
			{Label: "Redo", Role: RoleRedo, Accelerator: "cmd+shift+z"},
			{Separator: true},
			{Label: "Cut", Role: RoleCut, Accelerator: "cmd+x"},
			{Label: "Copy", Role: RoleCopy, Accelerator: "cmd+c"},
			{Label: "Paste", Role: RolePaste, Accelerator: "cmd+v"},
			{Label: "Select All", Role: RoleSelectAll, Accelerator: "cmd+a"},
		}},
	}
}
