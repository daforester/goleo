//go:build linux && !mobilebuild && !js

// Linux native menu bar, cgo-free via purego + GTK. glaze/webview_go put the
// WebKitWebView as the GtkWindow's only child, so a GtkMenuBar is added by
// reparenting: the webview moves into a vertical GtkBox under the menu bar.
// Built on the GTK main thread (via the window's Dispatch). Uses RTLD_NOLOAD so
// it binds the GTK already loaded by the webview backend, never a second copy.
//
// GTK3 only (webkit2gtk-4.0/4.1). GTK4 removed GtkMenuBar in favor of the GMenu
// model, so under GTK4 (webkitgtk-6.0) this logs and no-ops — use an HTML menu.

package runtime

import (
	"fmt"
	"log"
	"sync"

	"github.com/ebitengine/purego"
)

const menuRTLD = 0x0002 | 0x0004 // RTLD_NOW | RTLD_NOLOAD

var (
	menuGTKOnce  sync.Once
	menuGTKReady bool
	menuGTK3     bool

	gtkMenuBarNew         func() uintptr
	gtkMenuNew            func() uintptr
	gtkMenuItemNewLabel   func(label string) uintptr
	gtkSeparatorItemNew   func() uintptr
	gtkMenuItemSetSubmenu func(item, submenu uintptr)
	gtkMenuShellAppend    func(shell, child uintptr)
	gtkBinGetChild        func(bin uintptr) uintptr
	gtkContainerRemove    func(container, widget uintptr)
	gtkContainerAddMenu   func(container, widget uintptr)
	gtkBoxNew             func(orientation, spacing int32) uintptr
	gtkBoxPackStart       func(box, child uintptr, expand, fill bool, padding uint32)
	gtkWidgetShowAll      func(widget uintptr)
	gObjectRef            func(obj uintptr) uintptr
	gObjectUnref          func(obj uintptr)
	gSignalConnectMenu    func(instance uintptr, signal string, handler, data, destroy uintptr, flags int) uint64

	menuLinuxMu        sync.Mutex
	menuLinuxCallbacks []func()
	menuActivateCB     uintptr
	menuInstalled      bool
)

func loadGTKMenu() {
	menuGTKOnce.Do(func() {
		gtk := dlExisting("libgtk-4.so.1")
		if gtk != 0 {
			menuGTK3 = false // GTK4: GtkMenuBar is gone
			return
		}
		gtk = dlExisting("libgtk-3.so.0")
		if gtk == 0 {
			return
		}
		gobj := dlExisting("libgobject-2.0.so.0")
		if gobj == 0 {
			return
		}
		menuGTK3 = true
		purego.RegisterLibFunc(&gtkMenuBarNew, gtk, "gtk_menu_bar_new")
		purego.RegisterLibFunc(&gtkMenuNew, gtk, "gtk_menu_new")
		purego.RegisterLibFunc(&gtkMenuItemNewLabel, gtk, "gtk_menu_item_new_with_label")
		purego.RegisterLibFunc(&gtkSeparatorItemNew, gtk, "gtk_separator_menu_item_new")
		purego.RegisterLibFunc(&gtkMenuItemSetSubmenu, gtk, "gtk_menu_item_set_submenu")
		purego.RegisterLibFunc(&gtkMenuShellAppend, gtk, "gtk_menu_shell_append")
		purego.RegisterLibFunc(&gtkBinGetChild, gtk, "gtk_bin_get_child")
		purego.RegisterLibFunc(&gtkContainerRemove, gtk, "gtk_container_remove")
		purego.RegisterLibFunc(&gtkContainerAddMenu, gtk, "gtk_container_add")
		purego.RegisterLibFunc(&gtkBoxNew, gtk, "gtk_box_new")
		purego.RegisterLibFunc(&gtkBoxPackStart, gtk, "gtk_box_pack_start")
		purego.RegisterLibFunc(&gtkWidgetShowAll, gtk, "gtk_widget_show_all")
		purego.RegisterLibFunc(&gObjectRef, gobj, "g_object_ref")
		purego.RegisterLibFunc(&gObjectUnref, gobj, "g_object_unref")
		purego.RegisterLibFunc(&gSignalConnectMenu, gobj, "g_signal_connect_data")
		menuActivateCB = purego.NewCallback(func(widget, user uintptr) uintptr {
			idx := int(user)
			menuLinuxMu.Lock()
			var cb func()
			if idx >= 0 && idx < len(menuLinuxCallbacks) {
				cb = menuLinuxCallbacks[idx]
			}
			menuLinuxMu.Unlock()
			if cb != nil {
				cb()
			}
			return 0
		})
		menuGTKReady = true
	})
}

func dlExisting(name string) uintptr {
	h, err := purego.Dlopen(name, menuRTLD)
	if err != nil {
		return 0
	}
	return h
}

func (a *App) setNativeMenu(items []MenuItem) error {
	win := a.mainWin
	if win == nil || !win.IsValid() {
		return fmt.Errorf("goleo: SetMenu before the primary window exists")
	}
	loadGTKMenu()
	if !menuGTK3 {
		log.Println("goleo: native menu bar needs GTK3 (webkit2gtk-4.x); GTK4 is unsupported — use an HTML menu")
		return nil
	}
	if !menuGTKReady {
		return fmt.Errorf("goleo: GTK menu symbols unavailable")
	}
	win.Dispatch(func() { buildAndInstallGTKMenu(win, items) })
	return nil
}

func buildAndInstallGTKMenu(win *WebviewWindow, items []MenuItem) {
	window := uintptr(win.NativeHandle())
	if window == 0 || menuInstalled {
		return // reparenting twice would nest boxes; v1 installs once
	}
	menuLinuxMu.Lock()
	menuLinuxCallbacks = menuLinuxCallbacks[:0]
	menuLinuxMu.Unlock()

	bar := gtkMenuBarNew()
	for _, it := range items {
		if it.Separator || len(it.Submenu) == 0 {
			continue // top level = menus with submenus
		}
		top := gtkMenuItemNewLabel(it.Label)
		gtkMenuItemSetSubmenu(top, buildGTKSubmenu(it.Submenu))
		gtkMenuShellAppend(bar, top)
	}

	// Reparent: move the webview under a vbox with the menu bar on top.
	child := gtkBinGetChild(window)
	if child == 0 {
		return
	}
	gObjectRef(child)
	gtkContainerRemove(window, child)
	box := gtkBoxNew(1 /*GTK_ORIENTATION_VERTICAL*/, 0)
	gtkBoxPackStart(box, bar, false, false, 0)
	gtkBoxPackStart(box, child, true, true, 0)
	gtkContainerAddMenu(window, box)
	gObjectUnref(child)
	gtkWidgetShowAll(window)
	menuInstalled = true
}

func buildGTKSubmenu(items []MenuItem) uintptr {
	menu := gtkMenuNew()
	for _, it := range items {
		if it.Separator {
			gtkMenuShellAppend(menu, gtkSeparatorItemNew())
			continue
		}
		mi := gtkMenuItemNewLabel(it.Label)
		if len(it.Submenu) > 0 {
			gtkMenuItemSetSubmenu(mi, buildGTKSubmenu(it.Submenu))
		} else if cb := gtkMenuAction(it); cb != nil {
			menuLinuxMu.Lock()
			idx := len(menuLinuxCallbacks)
			menuLinuxCallbacks = append(menuLinuxCallbacks, cb)
			menuLinuxMu.Unlock()
			gSignalConnectMenu(mi, "activate", menuActivateCB, uintptr(idx), 0, 0)
		}
		gtkMenuShellAppend(menu, mi)
	}
	return menu
}

// gtkMenuAction resolves a role/OnClick to a Go func. WebKitGTK handles edit
// shortcuts itself; role items trigger the equivalent editing command.
func gtkMenuAction(it MenuItem) func() {
	return it.OnClick // roles on Linux are best-effort; custom OnClick is primary
}
