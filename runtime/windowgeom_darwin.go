//go:build darwin && !ios && !mobilebuild && !js

// macOS window geometry and chrome, cgo-free via purego + the objc runtime — the same
// approach tray_darwin.go and menu_darwin.go use, driven off the NSWindow that
// WebviewWindow.NativeHandle() returns.
//
// NOT VERIFIED ON HARDWARE. Written against the documented AppKit API and compile-checked
// by CI's macOS runner; the geometry values themselves need eyes on a Mac. The coordinate
// conversion below is the part most likely to be wrong, and it is called out in
// docs/roadmap.md rather than left for someone to discover.

package runtime

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// NSWindow styleMask bits.
const (
	nsWindowStyleTitled          = 1 << 0
	nsWindowStyleClosable        = 1 << 1
	nsWindowStyleMiniaturizable  = 1 << 2
	nsWindowStyleResizable       = 1 << 3
	nsWindowStyleFullScreenBit   = 1 << 14
	nsFloatingWindowLevel        = 3 // NSFloatingWindowLevel
	nsNormalWindowLevel          = 0
	nsFullScreenPrimaryCollectio = 1 << 7
)

type nsRect struct{ X, Y, W, H float64 }

func nsWindow(h unsafe.Pointer) objc.ID { return objc.ID(uintptr(h)) }

// screenHeight is the primary display's height, needed to convert between AppKit's
// bottom-left origin and the top-left origin WindowRect uses everywhere else.
func screenHeight() float64 {
	screen := objc.ID(objc.GetClass("NSScreen")).Send(objc.RegisterName("mainScreen"))
	if screen == 0 {
		return 0
	}
	f := objc.Send[nsRect](screen, objc.RegisterName("frame"))
	return f.H
}

func nativeGetRect(h unsafe.Pointer) (WindowRect, error) {
	w := nsWindow(h)
	if w == 0 {
		return WindowRect{}, fmt.Errorf("goleo: nil NSWindow")
	}
	f := objc.Send[nsRect](w, objc.RegisterName("frame"))
	sh := screenHeight()
	// AppKit's origin is bottom-left with Y up; WindowRect is top-left with Y down, so the
	// stored value means the same thing on every platform.
	top := sh - (f.Y + f.H)
	return WindowRect{X: int(f.X), Y: int(top), Width: int(f.W), Height: int(f.H)}, nil
}

func nativeSetRect(h unsafe.Pointer, r WindowRect) error {
	w := nsWindow(h)
	if w == 0 {
		return fmt.Errorf("goleo: nil NSWindow")
	}
	sh := screenHeight()
	frame := nsRect{
		X: float64(r.X),
		Y: sh - float64(r.Y) - float64(r.Height), // back to bottom-left
		W: float64(r.Width),
		H: float64(r.Height),
	}
	w.Send(objc.RegisterName("setFrame:display:"), frame, true)
	return nil
}

func nativeSetChrome(h unsafe.Pointer, c WindowChrome) error {
	w := nsWindow(h)
	if w == 0 {
		return fmt.Errorf("goleo: nil NSWindow")
	}

	if c.Resizable != nil || c.Decorations != nil {
		mask := uint64(objc.Send[uint64](w, objc.RegisterName("styleMask")))
		if c.Resizable != nil {
			if *c.Resizable {
				mask |= nsWindowStyleResizable
			} else {
				mask &^= nsWindowStyleResizable
			}
		}
		if c.Decorations != nil {
			deco := uint64(nsWindowStyleTitled | nsWindowStyleClosable | nsWindowStyleMiniaturizable)
			if *c.Decorations {
				mask |= deco
			} else {
				mask &^= deco
			}
		}
		w.Send(objc.RegisterName("setStyleMask:"), mask)
	}

	if c.AlwaysOnTop != nil {
		level := nsNormalWindowLevel
		if *c.AlwaysOnTop {
			level = nsFloatingWindowLevel
		}
		w.Send(objc.RegisterName("setLevel:"), level)
	}

	if c.Fullscreen != nil {
		// toggleFullScreen: is a TOGGLE, so calling it unconditionally would turn
		// fullscreen off when asked to turn it on. Check the current state first.
		mask := uint64(objc.Send[uint64](w, objc.RegisterName("styleMask")))
		isFull := mask&nsWindowStyleFullScreenBit != 0
		if isFull != *c.Fullscreen {
			w.Send(objc.RegisterName("toggleFullScreen:"), objc.ID(0))
		}
	}
	return nil
}

func nativeVisibleBounds() (WindowRect, error) {
	screen := objc.ID(objc.GetClass("NSScreen")).Send(objc.RegisterName("mainScreen"))
	if screen == 0 {
		return WindowRect{}, fmt.Errorf("goleo: no NSScreen")
	}
	// visibleFrame excludes the menu bar and Dock, which is what a restored window should
	// be clamped into — frame would allow a title bar under the menu bar.
	f := objc.Send[nsRect](screen, objc.RegisterName("visibleFrame"))
	return WindowRect{X: int(f.X), Y: 0, Width: int(f.W), Height: int(f.H)}, nil
}
