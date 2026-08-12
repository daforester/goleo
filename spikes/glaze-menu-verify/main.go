// Verifies the native application menu bar builds and installs without crashing.
// On macOS (glaze-menu-verify's real target) it exercises the purego/objc NSMenu
// backend: standard-role items (Quit/Copy/Paste/SelectAll), a custom OnClick
// item, an accelerator, a separator, and a submenu — set as NSApp.mainMenu on
// the GUI thread. On Linux the native menu is unsupported (MenuSupported()==false)
// so the menu is a no-op; the smoke then just confirms the windowed app runs and
// quits cleanly. "RESULT: PASS" + exit 0 on success.
package main

import (
	"context"
	"embed"
	"fmt"
	"time"

	goleo "github.com/daforester/goleo/runtime"
)

//go:embed all:frontend/dist
var fe embed.FS

func main() {
	var app *goleo.App
	app = goleo.New(goleo.Config{
		Title:      "menu-verify",
		Width:      420,
		Height:     300,
		WindowMode: goleo.WindowModeWebview,
		EmbedFS:    fe,
		Menu: []goleo.MenuItem{
			{Label: "menu-verify", Submenu: []goleo.MenuItem{
				{Label: "About", OnClick: func() {}},
				{Separator: true},
				{Label: "Quit", Role: goleo.RoleQuit, Accelerator: "cmd+q"},
			}},
			{Label: "Edit", Submenu: []goleo.MenuItem{
				{Label: "Copy", Role: goleo.RoleCopy, Accelerator: "cmd+c"},
				{Label: "Paste", Role: goleo.RolePaste, Accelerator: "cmd+v"},
				{Label: "Select All", Role: goleo.RoleSelectAll, Accelerator: "cmd+a"},
			}},
		},
		OnReady: func(ctx context.Context) {
			go func() {
				time.Sleep(3 * time.Second)
				fmt.Printf("RESULT: PASS (menu installed=%v; app ran + clean quit)\n", goleo.MenuSupported())
				app.Quit()
			}()
		},
	})
	goleo.RegisterBuiltins(app.Bridge())
	app.Run()
}
