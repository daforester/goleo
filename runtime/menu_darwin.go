//go:build darwin && !mobilebuild && !js

// macOS application menu bar, cgo-free via ebitengine/purego + the Objective-C
// runtime (NSMenu / NSMenuItem set as NSApplication.mainMenu). Standard Roles map
// to the native selectors sent up the responder chain (so Cmd+C/V/X/A/Z work in
// the WKWebView); custom items dispatch to Go through a registered objc target.
// Reuses the trSel/trClass/trStr objc helpers from tray_darwin.go.
//
// The menu is built on the main thread (Dispatched onto glaze's [NSApp run]
// loop). Status: cross-link verified; behavior checked on macos-14 via
// spikes/glaze-menu-verify.

package runtime

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ebitengine/purego/objc"
)

// NSEventModifierFlags
const (
	modShift   = 1 << 17
	modControl = 1 << 18
	modOption  = 1 << 19
	modCommand = 1 << 20
)

var (
	menuHandlerClass objc.Class
	menuHandlerOnce  sync.Once
	menuHandler      objc.ID
	menuCallbacks    []func() // indexed by NSMenuItem tag; rebuilt each SetMenu
)

func roleSelector(role MenuRole) objc.SEL {
	name := map[MenuRole]string{
		RoleQuit:      "terminate:",
		RoleUndo:      "undo:",
		RoleRedo:      "redo:",
		RoleCut:       "cut:",
		RoleCopy:      "copy:",
		RolePaste:     "paste:",
		RoleSelectAll: "selectAll:",
		RoleMinimize:  "performMiniaturize:",
		RoleClose:     "performClose:",
	}[role]
	if name == "" {
		return 0
	}
	return traySel(name)
}

// parseAccel turns "cmd+shift+z" into ("z", modmask). Empty accel -> ("", 0).
func parseAccel(accel string) (string, uint) {
	if accel == "" {
		return "", 0
	}
	var mask uint
	key := ""
	for _, tok := range strings.Split(strings.ToLower(accel), "+") {
		switch strings.TrimSpace(tok) {
		case "cmd", "command", "super":
			mask |= modCommand
		case "shift":
			mask |= modShift
		case "alt", "option", "opt":
			mask |= modOption
		case "ctrl", "control":
			mask |= modControl
		case "":
		default:
			key = strings.TrimSpace(tok)
		}
	}
	return key, mask
}

func (a *App) setNativeMenu(items []MenuItem) error {
	win := a.mainWin
	if win == nil || !win.IsValid() {
		return fmt.Errorf("goleo: SetMenu before the primary window exists")
	}
	// Build + install on the main thread (fire-and-forget: the menu bar updates
	// live; blocking here would deadlock during startup, before Run pumps).
	win.Dispatch(func() { buildAndInstallMenu(items) })
	return nil
}

func buildAndInstallMenu(items []MenuItem) {
	menuHandlerOnce.Do(func() {
		menuHandlerClass, _ = objc.RegisterClass(
			"GoleoMenuHandler", objc.GetClass("NSObject"), nil, nil,
			[]objc.MethodDef{{
				Cmd: traySel("goleoMenuClicked:"),
				Fn: func(self objc.ID, _cmd objc.SEL, sender objc.ID) {
					tag := int(sender.Send(traySel("tag")))
					if tag >= 0 && tag < len(menuCallbacks) && menuCallbacks[tag] != nil {
						menuCallbacks[tag]()
					}
				},
			}},
		)
	})
	if menuHandler == 0 {
		menuHandler = objc.ID(menuHandlerClass).Send(traySel("new"))
	}
	menuCallbacks = menuCallbacks[:0]

	app := trayClass("NSApplication").Send(traySel("sharedApplication"))
	app.Send(traySel("setMainMenu:"), buildMenu(items))
}

func buildMenu(items []MenuItem) objc.ID {
	menu := trayClass("NSMenu").Send(traySel("alloc")).Send(traySel("init"))
	for _, it := range items {
		var mi objc.ID
		if it.Separator {
			mi = trayClass("NSMenuItem").Send(traySel("separatorItem"))
			menu.Send(traySel("addItem:"), mi)
			continue
		}

		key, mask := parseAccel(it.Accelerator)
		var action objc.SEL
		tag := -1
		if sel := roleSelector(it.Role); sel != 0 {
			action = sel // sent up the responder chain (nil target)
		} else if it.OnClick != nil {
			action = traySel("goleoMenuClicked:")
			tag = len(menuCallbacks)
			menuCallbacks = append(menuCallbacks, it.OnClick)
		}

		mi = trayClass("NSMenuItem").Send(traySel("alloc")).Send(
			traySel("initWithTitle:action:keyEquivalent:"),
			trayStr(it.Label), action, trayStr(key))
		if key != "" {
			mi.Send(traySel("setKeyEquivalentModifierMask:"), mask)
		}
		if tag >= 0 {
			mi.Send(traySel("setTarget:"), menuHandler)
			mi.Send(traySel("setTag:"), tag)
		}
		if len(it.Submenu) > 0 {
			sub := buildMenu(it.Submenu)
			sub.Send(traySel("setTitle:"), trayStr(it.Label))
			mi.Send(traySel("setSubmenu:"), sub)
		}
		menu.Send(traySel("addItem:"), mi)
	}
	return menu
}
