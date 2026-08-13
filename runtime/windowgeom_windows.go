//go:build windows && !mobilebuild && !js

// Windows window geometry and chrome, cgo-free via purego + user32 — the same approach
// menu_windows.go uses for the native menu bar, driven off the HWND that
// WebviewWindow.NativeHandle() returns.

package runtime

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	gwlStyle   = -16
	gwlExStyle = -20

	wsThickFrame  = 0x00040000 // resizable border
	wsMaximizeBox = 0x00010000
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsPopup       = 0x80000000

	wsExTopmost = 0x00000008

	swpNoMove       = 0x0002
	swpNoSize       = 0x0001
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020

	hwndTopmost   = ^uintptr(0)     // (HWND)-1
	hwndNoTopmost = ^uintptr(0) - 1 // (HWND)-2

	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	swShowMaximized = 3
	swRestore       = 9
)

type winRECT struct{ Left, Top, Right, Bottom int32 }

var (
	geomOnce            sync.Once
	pGetWindowRect      func(hwnd uintptr, r *winRECT) bool
	pSetWindowPos       func(hwnd, insertAfter uintptr, x, y, cx, cy int32, flags uint32) bool
	pGetWindowLongPtrW  func(hwnd uintptr, index int32) uintptr
	pSetWindowLongPtrW2 func(hwnd uintptr, index int32, v uintptr) uintptr
	pGetSystemMetrics   func(index int32) int32
	pShowWindow         func(hwnd uintptr, cmd int32) bool
)

func loadGeomUser32() {
	geomOnce.Do(func() {
		// purego.Dlopen is Unix-only; on Windows load the DLL via syscall and hand purego
		// the handle, matching loadUser32 in menu_windows.go.
		h, err := syscall.LoadLibrary("user32.dll")
		if err != nil {
			return
		}
		u32 := uintptr(h)
		purego.RegisterLibFunc(&pGetWindowRect, u32, "GetWindowRect")
		purego.RegisterLibFunc(&pSetWindowPos, u32, "SetWindowPos")
		purego.RegisterLibFunc(&pGetWindowLongPtrW, u32, "GetWindowLongPtrW")
		purego.RegisterLibFunc(&pSetWindowLongPtrW2, u32, "SetWindowLongPtrW")
		purego.RegisterLibFunc(&pGetSystemMetrics, u32, "GetSystemMetrics")
		purego.RegisterLibFunc(&pShowWindow, u32, "ShowWindow")
	})
}

func nativeGetRect(h unsafe.Pointer) (WindowRect, error) {
	loadGeomUser32()
	if pGetWindowRect == nil {
		return WindowRect{}, fmt.Errorf("goleo: user32 unavailable")
	}
	var r winRECT
	if !pGetWindowRect(uintptr(h), &r) {
		return WindowRect{}, fmt.Errorf("goleo: GetWindowRect failed")
	}
	return WindowRect{
		X:      int(r.Left),
		Y:      int(r.Top),
		Width:  int(r.Right - r.Left),
		Height: int(r.Bottom - r.Top),
	}, nil
}

func nativeSetRect(h unsafe.Pointer, rect WindowRect) error {
	loadGeomUser32()
	if pSetWindowPos == nil {
		return fmt.Errorf("goleo: user32 unavailable")
	}
	ok := pSetWindowPos(uintptr(h), 0,
		int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height),
		swpNoZOrder|swpNoActivate)
	if !ok {
		return fmt.Errorf("goleo: SetWindowPos failed")
	}
	return nil
}

func nativeSetChrome(h unsafe.Pointer, c WindowChrome) error {
	loadGeomUser32()
	if pGetWindowLongPtrW == nil {
		return fmt.Errorf("goleo: user32 unavailable")
	}
	hwnd := uintptr(h)

	if c.Resizable != nil || c.Decorations != nil {
		style := pGetWindowLongPtrW(hwnd, gwlStyle)
		if c.Resizable != nil {
			// The maximize box goes with the resize border: leaving it enabled on a
			// non-resizable window gives a button that resizes a window the app said
			// could not be resized.
			if *c.Resizable {
				style |= wsThickFrame | wsMaximizeBox
			} else {
				style &^= wsThickFrame | wsMaximizeBox
			}
		}
		if c.Decorations != nil {
			if *c.Decorations {
				style |= wsCaption | wsSysMenu
				style &^= wsPopup
			} else {
				style &^= wsCaption | wsSysMenu
				style |= wsPopup
			}
		}
		pSetWindowLongPtrW2(hwnd, gwlStyle, style)
		// A style change is not visible until the frame is recalculated; without
		// SWP_FRAMECHANGED the old border is still drawn until something else forces a
		// repaint, which reads as "the call did nothing".
		pSetWindowPos(hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpFrameChanged)
	}

	if c.AlwaysOnTop != nil {
		after := hwndNoTopmost
		if *c.AlwaysOnTop {
			after = hwndTopmost
		}
		pSetWindowPos(hwnd, after, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	}

	if c.Fullscreen != nil {
		// Deliberately maximize rather than a true borderless-fullscreen state change.
		// Real fullscreen means stripping the frame, saving the previous placement and
		// restoring it exactly, and getting that wrong strands the window with no border
		// and no way back. Maximize is the honest 90% and is reversible; borderless
		// fullscreen belongs with a proper save/restore of the pre-fullscreen rect.
		cmd := int32(swRestore)
		if *c.Fullscreen {
			cmd = swShowMaximized
		}
		pShowWindow(hwnd, cmd)
	}
	return nil
}

func nativeVisibleBounds() (WindowRect, error) {
	loadGeomUser32()
	if pGetSystemMetrics == nil {
		return WindowRect{}, fmt.Errorf("goleo: user32 unavailable")
	}
	// The VIRTUAL screen is the union of all monitors, which is what clamping needs — a
	// primary-monitor-only bound would refuse to restore a window that was legitimately
	// saved on a second display that is still attached.
	return WindowRect{
		X:      int(pGetSystemMetrics(smXVirtualScreen)),
		Y:      int(pGetSystemMetrics(smYVirtualScreen)),
		Width:  int(pGetSystemMetrics(smCXVirtualScreen)),
		Height: int(pGetSystemMetrics(smCYVirtualScreen)),
	}, nil
}
