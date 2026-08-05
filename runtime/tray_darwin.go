//go:build darwin && !mobilebuild && !js

// macOS system tray, cgo-free via ebitengine/purego + the Objective-C runtime —
// the SAME FFI glaze uses, so it shares glaze's single `fakecgo` and never pulls
// in gogpu/systray → go-webgpu/goffi (whose separate fakecgo collides with
// glaze's `_cgo_init` at Mach-O link time; see SPIKES.md). Windows/Linux keep
// gogpu/systray (tray_desktop.go).
//
// Builds an NSStatusItem in the menu bar with an optional icon/tooltip and a
// menu whose items dispatch back to Go. Runs under a menu-bar-only NSApplication
// (accessory activation policy, no Dock icon). Invoked from App.Run in
// Background mode on the locked main goroutine (AppKit is main-thread-only).
//
// Status: cross-compile/link verified; interactive behavior is hardware-gated
// (verified on macos-14 via the tray smoke in glaze-verify.yml).

package runtime

import (
	"os"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

const (
	nsApplicationActivationPolicyAccessory = 1    // menu-bar only, no Dock icon
	nsVariableStatusItemLength             = -1.0 // auto-size to content
)

var (
	traySelCache     sync.Map // string -> objc.SEL
	trayHandlerClass objc.Class
	trayHandlerOnce  sync.Once
	trayHandler      objc.ID  // retained for the lifetime of the app (menu target)
	trayCallbacks    []func() // indexed by NSMenuItem tag
)

func traySel(name string) objc.SEL {
	if v, ok := traySelCache.Load(name); ok {
		return v.(objc.SEL)
	}
	s := objc.RegisterName(name)
	traySelCache.Store(name, s)
	return s
}

func trayClass(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

func trayStr(s string) objc.ID {
	return trayClass("NSString").Send(traySel("stringWithUTF8String:"), s)
}

// runTrayLoop builds the status item + menu and runs the AppKit loop; it blocks.
// Quit is handled by the watcher goroutine, which shuts down and exits the
// process (matching tray_desktop.go — [NSApp run] does not return on cancel).
func (a *App) runTrayLoop() {
	go func() {
		<-a.ctx.Done()
		a.shutdown()
		os.Exit(0)
	}()

	app := trayClass("NSApplication").Send(traySel("sharedApplication"))
	app.Send(traySel("setActivationPolicy:"), nsApplicationActivationPolicyAccessory)

	statusBar := trayClass("NSStatusBar").Send(traySel("systemStatusBar"))
	item := statusBar.Send(traySel("statusItemWithLength:"), float64(nsVariableStatusItemLength))
	button := item.Send(traySel("button"))

	if cfg := a.config.Tray; cfg != nil {
		if len(cfg.Icon) > 0 {
			data := trayClass("NSData").Send(traySel("dataWithBytes:length:"),
				unsafe.Pointer(&cfg.Icon[0]), len(cfg.Icon))
			if img := trayClass("NSImage").Send(traySel("alloc")).Send(traySel("initWithData:"), data); img != 0 {
				img.Send(traySel("setTemplate:"), true) // adapt to light/dark menu bar
				button.Send(traySel("setImage:"), img)
			}
		} else if cfg.Tooltip != "" {
			button.Send(traySel("setTitle:"), trayStr(cfg.Tooltip))
		}
		if cfg.Tooltip != "" {
			button.Send(traySel("setToolTip:"), trayStr(cfg.Tooltip))
		}

		if len(cfg.Items) > 0 {
			trayHandlerOnce.Do(func() {
				trayHandlerClass, _ = objc.RegisterClass(
					"GoleoTrayHandler", objc.GetClass("NSObject"), nil, nil,
					[]objc.MethodDef{{
						Cmd: traySel("goleoItemClicked:"),
						Fn: func(self objc.ID, _cmd objc.SEL, sender objc.ID) {
							tag := int(sender.Send(traySel("tag")))
							if tag >= 0 && tag < len(trayCallbacks) && trayCallbacks[tag] != nil {
								trayCallbacks[tag]()
							}
						},
					}},
				)
			})
			trayHandler = objc.ID(trayHandlerClass).Send(traySel("new"))

			menu := trayClass("NSMenu").Send(traySel("alloc")).Send(traySel("init"))
			trayCallbacks = make([]func(), len(cfg.Items))
			for i, it := range cfg.Items {
				trayCallbacks[i] = it.OnClick
				mi := trayClass("NSMenuItem").Send(traySel("alloc")).Send(
					traySel("initWithTitle:action:keyEquivalent:"),
					trayStr(it.Label), traySel("goleoItemClicked:"), trayStr(""))
				mi.Send(traySel("setTarget:"), trayHandler)
				mi.Send(traySel("setTag:"), i)
				menu.Send(traySel("addItem:"), mi)
			}
			item.Send(traySel("setMenu:"), menu)
		}
	}

	app.Send(traySel("activateIgnoringOtherApps:"), true)
	app.Send(traySel("run")) // blocks; process exits via the watcher's os.Exit
}
