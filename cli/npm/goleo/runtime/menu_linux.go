//go:build linux && !mobilebuild && !js

// Linux native menu bar, cgo-free via purego + GTK. glaze/webview_go put the
// WebKitWebView as the GtkWindow's only child, so the menu bar is added by
// reparenting the webview under a vertical GtkBox. Built on the GTK main thread
// (via the window's Dispatch). RTLD_NOLOAD binds the GTK already loaded by the
// webview backend, never a second copy.
//
//   - GTK3 (webkit2gtk-4.0/4.1): GtkMenuBar + GtkMenuItem, "activate" signals,
//     accelerators via a GtkAccelGroup on the window.
//   - GTK4 (webkitgtk-6.0): GtkMenuBar was removed, so it uses the GMenu model +
//     GtkPopoverMenuBar + GActions inserted on the window (prefix "menu").

package runtime

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
)

const menuRTLD = 0x0002 | 0x0004 // RTLD_NOW | RTLD_NOLOAD

var (
	menuGTKOnce   sync.Once
	menuGTK3      bool
	menuGTK4      bool
	menuInstalled bool

	// shared
	gObjectRef         func(obj uintptr) uintptr
	gObjectUnref       func(obj uintptr)
	gSignalConnectMenu func(instance uintptr, signal string, handler, data, destroy uintptr, flags int) uint64

	// GTK3
	gtkMenuBarNew         func() uintptr
	gtkMenuNew            func() uintptr
	gtkMenuItemNewLabel   func(label string) uintptr
	gtkSeparatorItemNew   func() uintptr
	gtkMenuItemSetSubmenu func(item, submenu uintptr)
	gtkMenuShellAppend    func(shell, child uintptr)
	gtkBinGetChild        func(bin uintptr) uintptr
	gtkContainerRemove    func(container, widget uintptr)
	gtkContainerAdd       func(container, widget uintptr)
	gtkBoxNew             func(orientation, spacing int32) uintptr
	gtkBoxPackStart       func(box, child uintptr, expand, fill bool, padding uint32)
	gtkWidgetShowAll      func(widget uintptr)
	gtkAccelGroupNew      func() uintptr
	gtkWindowAddAccelGrp  func(window, group uintptr)
	gtkWidgetAddAccel     func(widget uintptr, sig string, group uintptr, key uint32, mods uint32, flags uint32)

	// GTK4
	gtkPopoverMenuBarNew  func(model uintptr) uintptr
	gtkWindowGetChild     func(window uintptr) uintptr
	gtkWindowSetChild     func(window, child uintptr)
	gtkBoxAppend          func(box, child uintptr)
	gtkWidgetInsertGroup  func(widget uintptr, prefix string, group uintptr)
	gMenuNew              func() uintptr
	gMenuAppend           func(menu uintptr, label, action string)
	gMenuAppendSubmenu    func(menu uintptr, label string, submenu uintptr)
	gMenuAppendSection    func(menu, label, section uintptr)
	gSimpleActionGroupNew func() uintptr
	gSimpleActionNew      func(name string, paramType uintptr) uintptr
	gActionMapAddAction   func(actionMap, action uintptr)

	menuLinuxMu        sync.Mutex
	menuLinuxCallbacks []func()
	menuActivateCB     uintptr // GTK3 GtkMenuItem "activate": (item, user)
	menuActionCB       uintptr // GTK4 GAction "activate": (action, param, user)
	menuAccelGroup     uintptr
	gtk4ActionGroup    uintptr
	gtk4ActionSeq      int
)

func dlExisting(name string) uintptr {
	h, err := purego.Dlopen(name, menuRTLD)
	if err != nil {
		return 0
	}
	return h
}

func loadGTKMenu() {
	menuGTKOnce.Do(func() {
		gobj := dlExisting("libgobject-2.0.so.0")
		if gobj == 0 {
			return
		}
		purego.RegisterLibFunc(&gObjectRef, gobj, "g_object_ref")
		purego.RegisterLibFunc(&gObjectUnref, gobj, "g_object_unref")
		purego.RegisterLibFunc(&gSignalConnectMenu, gobj, "g_signal_connect_data")

		if gtk := dlExisting("libgtk-4.so.1"); gtk != 0 {
			gio := dlExisting("libgio-2.0.so.0")
			if gio == 0 {
				return
			}
			purego.RegisterLibFunc(&gtkPopoverMenuBarNew, gtk, "gtk_popover_menu_bar_new_from_model")
			purego.RegisterLibFunc(&gtkWindowGetChild, gtk, "gtk_window_get_child")
			purego.RegisterLibFunc(&gtkWindowSetChild, gtk, "gtk_window_set_child")
			purego.RegisterLibFunc(&gtkBoxNew, gtk, "gtk_box_new")
			purego.RegisterLibFunc(&gtkBoxAppend, gtk, "gtk_box_append")
			purego.RegisterLibFunc(&gtkWidgetInsertGroup, gtk, "gtk_widget_insert_action_group")
			purego.RegisterLibFunc(&gMenuNew, gio, "g_menu_new")
			purego.RegisterLibFunc(&gMenuAppend, gio, "g_menu_append")
			purego.RegisterLibFunc(&gMenuAppendSubmenu, gio, "g_menu_append_submenu")
			purego.RegisterLibFunc(&gMenuAppendSection, gio, "g_menu_append_section")
			purego.RegisterLibFunc(&gSimpleActionGroupNew, gio, "g_simple_action_group_new")
			purego.RegisterLibFunc(&gSimpleActionNew, gio, "g_simple_action_new")
			purego.RegisterLibFunc(&gActionMapAddAction, gio, "g_action_map_add_action")
			menuActionCB = purego.NewCallback(func(action, param, user uintptr) uintptr {
				dispatchMenuCallback(int(user))
				return 0
			})
			menuGTK4 = true
			return
		}

		gtk := dlExisting("libgtk-3.so.0")
		if gtk == 0 {
			return
		}
		purego.RegisterLibFunc(&gtkMenuBarNew, gtk, "gtk_menu_bar_new")
		purego.RegisterLibFunc(&gtkMenuNew, gtk, "gtk_menu_new")
		purego.RegisterLibFunc(&gtkMenuItemNewLabel, gtk, "gtk_menu_item_new_with_label")
		purego.RegisterLibFunc(&gtkSeparatorItemNew, gtk, "gtk_separator_menu_item_new")
		purego.RegisterLibFunc(&gtkMenuItemSetSubmenu, gtk, "gtk_menu_item_set_submenu")
		purego.RegisterLibFunc(&gtkMenuShellAppend, gtk, "gtk_menu_shell_append")
		purego.RegisterLibFunc(&gtkBinGetChild, gtk, "gtk_bin_get_child")
		purego.RegisterLibFunc(&gtkContainerRemove, gtk, "gtk_container_remove")
		purego.RegisterLibFunc(&gtkContainerAdd, gtk, "gtk_container_add")
		purego.RegisterLibFunc(&gtkBoxNew, gtk, "gtk_box_new")
		purego.RegisterLibFunc(&gtkBoxPackStart, gtk, "gtk_box_pack_start")
		purego.RegisterLibFunc(&gtkWidgetShowAll, gtk, "gtk_widget_show_all")
		purego.RegisterLibFunc(&gtkAccelGroupNew, gtk, "gtk_accel_group_new")
		purego.RegisterLibFunc(&gtkWindowAddAccelGrp, gtk, "gtk_window_add_accel_group")
		purego.RegisterLibFunc(&gtkWidgetAddAccel, gtk, "gtk_widget_add_accelerator")
		menuActivateCB = purego.NewCallback(func(widget, user uintptr) uintptr {
			dispatchMenuCallback(int(user))
			return 0
		})
		menuGTK3 = true
	})
}

func dispatchMenuCallback(idx int) {
	menuLinuxMu.Lock()
	var cb func()
	if idx >= 0 && idx < len(menuLinuxCallbacks) {
		cb = menuLinuxCallbacks[idx]
	}
	menuLinuxMu.Unlock()
	if cb != nil {
		cb()
	}
}

func (a *App) setNativeMenu(items []MenuItem) error {
	win := a.mainWin
	if win == nil || !win.IsValid() {
		return fmt.Errorf("goleo: SetMenu before the primary window exists")
	}
	loadGTKMenu()
	if !menuGTK3 && !menuGTK4 {
		return fmt.Errorf("goleo: GTK menu symbols unavailable")
	}
	win.Dispatch(func() {
		if menuInstalled {
			return // reparenting twice would nest boxes; v1 installs once
		}
		window := uintptr(win.NativeHandle())
		if window == 0 {
			return
		}
		menuLinuxMu.Lock()
		menuLinuxCallbacks = menuLinuxCallbacks[:0]
		menuLinuxMu.Unlock()
		if menuGTK4 {
			installGTK4Menu(window, items)
		} else {
			installGTK3Menu(window, items)
		}
		menuInstalled = true
	})
	return nil
}

// --- GTK3 ---

func installGTK3Menu(window uintptr, items []MenuItem) {
	menuAccelGroup = gtkAccelGroupNew()
	gtkWindowAddAccelGrp(window, menuAccelGroup)

	bar := gtkMenuBarNew()
	for _, it := range items {
		if it.Separator || len(it.Submenu) == 0 {
			continue
		}
		top := gtkMenuItemNewLabel(it.Label)
		gtkMenuItemSetSubmenu(top, buildGTK3Submenu(it.Submenu))
		gtkMenuShellAppend(bar, top)
	}

	child := gtkBinGetChild(window)
	if child == 0 {
		return
	}
	gObjectRef(child)
	gtkContainerRemove(window, child)
	box := gtkBoxNew(1 /*VERTICAL*/, 0)
	gtkBoxPackStart(box, bar, false, false, 0)
	gtkBoxPackStart(box, child, true, true, 0)
	gtkContainerAdd(window, box)
	gObjectUnref(child)
	gtkWidgetShowAll(window)
}

func buildGTK3Submenu(items []MenuItem) uintptr {
	menu := gtkMenuNew()
	for _, it := range items {
		if it.Separator {
			gtkMenuShellAppend(menu, gtkSeparatorItemNew())
			continue
		}
		mi := gtkMenuItemNewLabel(it.Label)
		if len(it.Submenu) > 0 {
			gtkMenuItemSetSubmenu(mi, buildGTK3Submenu(it.Submenu))
		} else if cb := it.OnClick; cb != nil {
			menuLinuxMu.Lock()
			idx := len(menuLinuxCallbacks)
			menuLinuxCallbacks = append(menuLinuxCallbacks, cb)
			menuLinuxMu.Unlock()
			gSignalConnectMenu(mi, "activate", menuActivateCB, uintptr(idx), 0, 0)
		}
		if key, mods := gtkAccel(it.Accelerator); key != 0 {
			gtkWidgetAddAccel(mi, "activate", menuAccelGroup, key, mods, 1 /*GTK_ACCEL_VISIBLE*/)
		}
		gtkMenuShellAppend(menu, mi)
	}
	return menu
}

// --- GTK4 ---

func installGTK4Menu(window uintptr, items []MenuItem) {
	gtk4ActionGroup = gSimpleActionGroupNew()
	gtk4ActionSeq = 0
	barModel := gMenuNew()
	for _, it := range items {
		if it.Separator || len(it.Submenu) == 0 {
			continue
		}
		gMenuAppendSubmenu(barModel, it.Label, buildGTK4Model(it.Submenu))
	}
	gtkWidgetInsertGroup(window, "menu", gtk4ActionGroup)
	bar := gtkPopoverMenuBarNew(barModel)

	child := gtkWindowGetChild(window)
	if child == 0 {
		return
	}
	gObjectRef(child)
	gtkWindowSetChild(window, 0)
	box := gtkBoxNew(1 /*VERTICAL*/, 0)
	gtkBoxAppend(box, bar)
	gtkBoxAppend(box, child)
	gtkWindowSetChild(window, box)
	gObjectUnref(child)
}

// gtkAccel parses "cmd+shift+z" into a GDK keyval + GdkModifierType mask (GTK3).
// "cmd" maps to Control on Linux. Returns (0,0) if there's no key.
func gtkAccel(accel string) (uint32, uint32) {
	if accel == "" {
		return 0, 0
	}
	var key, mods uint32
	for _, tok := range strings.Split(strings.ToLower(accel), "+") {
		switch strings.TrimSpace(tok) {
		case "cmd", "command", "ctrl", "control":
			mods |= 1 << 2 // GDK_CONTROL_MASK
		case "shift":
			mods |= 1 << 0 // GDK_SHIFT_MASK
		case "alt", "option", "opt":
			mods |= 1 << 3 // GDK_MOD1_MASK
		case "super":
			mods |= 1 << 26 // GDK_SUPER_MASK
		case "":
		default:
			if t := strings.TrimSpace(tok); t != "" {
				key = uint32(t[0]) // a-z / 0-9 GDK keyvals are their ASCII codes
			}
		}
	}
	return key, mods
}

func buildGTK4Model(items []MenuItem) uintptr {
	model := gMenuNew()
	section := gMenuNew()
	nonEmpty := false
	flush := func() {
		if nonEmpty {
			gMenuAppendSection(model, 0, section)
			section = gMenuNew()
			nonEmpty = false
		}
	}
	for _, it := range items {
		if it.Separator {
			flush()
			continue
		}
		if len(it.Submenu) > 0 {
			gMenuAppendSubmenu(section, it.Label, buildGTK4Model(it.Submenu))
			nonEmpty = true
			continue
		}
		name := fmt.Sprintf("i%d", gtk4ActionSeq)
		gtk4ActionSeq++
		action := gSimpleActionNew(name, 0)
		if it.OnClick != nil {
			menuLinuxMu.Lock()
			idx := len(menuLinuxCallbacks)
			menuLinuxCallbacks = append(menuLinuxCallbacks, it.OnClick)
			menuLinuxMu.Unlock()
			gSignalConnectMenu(action, "activate", menuActionCB, uintptr(idx), 0, 0)
		}
		gActionMapAddAction(gtk4ActionGroup, action)
		gMenuAppend(section, it.Label, "menu."+name)
		nonEmpty = true
	}
	flush()
	return model
}
