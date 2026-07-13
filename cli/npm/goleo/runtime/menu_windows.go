//go:build windows && !mobilebuild && !js

// Windows native menu bar, cgo-free via purego + user32. Builds an HMENU tree,
// SetMenu()s it on the WebView2 window's HWND, and subclasses the window proc to
// receive WM_COMMAND clicks — dispatching to Go. WebView2 already handles the
// standard edit keyboard shortcuts (Ctrl+C/V/X/A/Z) internally, so menu roles
// are convenience actions (Quit + execCommand) rather than shortcut plumbing;
// real accelerator tables aren't installed (the webview owns the message loop).

package runtime

import (
	"fmt"
	"sync"
	"syscall"

	"github.com/ebitengine/purego"
)

const (
	mfString    = 0x0000
	mfPopup     = 0x0010
	mfSeparator = 0x0800
	gwlpWndProc = -4
	wmCommand   = 0x0111
)

var (
	user32Once         sync.Once
	pCreateMenu        func() uintptr
	pCreatePopupMenu   func() uintptr
	pAppendMenuW       func(hMenu uintptr, flags uint32, id uintptr, item *uint16) bool
	pSetMenu           func(hwnd, hMenu uintptr) bool
	pDrawMenuBar       func(hwnd uintptr) bool
	pSetWindowLongPtrW func(hwnd uintptr, index int32, newLong uintptr) uintptr
	pCallWindowProcW   func(prev, hwnd, msg, wparam, lparam uintptr) uintptr

	winMenuMu        sync.Mutex
	winMenuCallbacks map[uintptr]func()
	winMenuNextID    uintptr
	winProcOnce      sync.Once
	winOldProc       uintptr
	winProcCB        uintptr
)

func loadUser32() {
	user32Once.Do(func() {
		// purego.Dlopen is Unix-only; on Windows load the DLL via syscall and
		// hand purego the handle (it resolves symbols with GetProcAddress).
		h, err := syscall.LoadLibrary("user32.dll")
		if err != nil {
			return
		}
		u32 := uintptr(h)
		purego.RegisterLibFunc(&pCreateMenu, u32, "CreateMenu")
		purego.RegisterLibFunc(&pCreatePopupMenu, u32, "CreatePopupMenu")
		purego.RegisterLibFunc(&pAppendMenuW, u32, "AppendMenuW")
		purego.RegisterLibFunc(&pSetMenu, u32, "SetMenu")
		purego.RegisterLibFunc(&pDrawMenuBar, u32, "DrawMenuBar")
		purego.RegisterLibFunc(&pSetWindowLongPtrW, u32, "SetWindowLongPtrW")
		purego.RegisterLibFunc(&pCallWindowProcW, u32, "CallWindowProcW")
	})
}

func (a *App) setNativeMenu(items []MenuItem) error {
	win := a.mainWin
	if win == nil || !win.IsValid() {
		return fmt.Errorf("goleo: SetMenu before the primary window exists")
	}
	loadUser32()
	if pSetMenu == nil {
		return fmt.Errorf("goleo: user32 menu functions unavailable")
	}
	// Menu + wndproc calls must run on the window's UI thread.
	win.Dispatch(func() { buildAndSetWin32Menu(a, win, items) })
	return nil
}

func buildAndSetWin32Menu(a *App, win *WebviewWindow, items []MenuItem) {
	hwnd := uintptr(win.NativeHandle())
	if hwnd == 0 {
		return
	}
	winMenuMu.Lock()
	winMenuCallbacks = map[uintptr]func(){}
	winMenuNextID = 1
	winMenuMu.Unlock()

	bar := pCreateMenu()
	for _, it := range items {
		if it.Separator {
			continue // separators are meaningless at the top bar level
		}
		sub := buildWin32Submenu(a, win, it.Submenu)
		pAppendMenuW(bar, mfPopup, sub, utf16(it.Label))
	}

	// Subclass the window proc once to receive WM_COMMAND for menu clicks.
	winProcOnce.Do(func() {
		winProcCB = purego.NewCallback(func(h, msg, wparam, lparam uintptr) uintptr {
			if msg == wmCommand {
				id := wparam & 0xFFFF
				winMenuMu.Lock()
				cb := winMenuCallbacks[id]
				winMenuMu.Unlock()
				if cb != nil {
					cb()
					return 0
				}
			}
			return pCallWindowProcW(winOldProc, h, msg, wparam, lparam)
		})
		winOldProc = pSetWindowLongPtrW(hwnd, gwlpWndProc, winProcCB)
	})

	pSetMenu(hwnd, bar)
	pDrawMenuBar(hwnd)
}

func buildWin32Submenu(a *App, win *WebviewWindow, items []MenuItem) uintptr {
	menu := pCreatePopupMenu()
	for _, it := range items {
		if it.Separator {
			pAppendMenuW(menu, mfSeparator, 0, nil)
			continue
		}
		if len(it.Submenu) > 0 {
			pAppendMenuW(menu, mfPopup, buildWin32Submenu(a, win, it.Submenu), utf16(it.Label))
			continue
		}
		action := winMenuAction(a, win, it)
		winMenuMu.Lock()
		id := winMenuNextID
		winMenuNextID++
		winMenuCallbacks[id] = action
		winMenuMu.Unlock()
		pAppendMenuW(menu, mfString, id, utf16(it.Label))
	}
	return menu
}

// winMenuAction maps a role or OnClick to a Go func. WebView2 handles edit
// shortcuts natively; role menu items invoke the equivalent via execCommand.
func winMenuAction(a *App, win *WebviewWindow, it MenuItem) func() {
	switch it.Role {
	case RoleQuit:
		return a.Quit
	case RoleCopy:
		return func() { win.Eval("document.execCommand('copy')") }
	case RolePaste:
		return func() { win.Eval("document.execCommand('paste')") }
	case RoleCut:
		return func() { win.Eval("document.execCommand('cut')") }
	case RoleSelectAll:
		return func() { win.Eval("document.execCommand('selectAll')") }
	case RoleUndo:
		return func() { win.Eval("document.execCommand('undo')") }
	case RoleRedo:
		return func() { win.Eval("document.execCommand('redo')") }
	}
	return it.OnClick
}

func utf16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		z := uint16(0)
		return &z
	}
	return p
}
