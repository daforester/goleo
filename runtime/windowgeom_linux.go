//go:build linux && !android && !mobilebuild && !js

// Linux window geometry and chrome, cgo-free via purego + GTK3 — driven off the GtkWindow*
// that WebviewWindow.NativeHandle() returns, the same handle
// webview_glaze_permissions_linux.go uses for the permission signal.
//
// NOT VERIFIED ON HARDWARE. Compile-checked only.
//
// A caveat that is not a bug and should not be "fixed": under WAYLAND, a client cannot
// position its own windows. gtk_window_move is a no-op there by design — the compositor
// owns placement. So SetRect's move component silently does nothing on Wayland while the
// resize works, and a restored window comes back the right SIZE at a compositor-chosen
// position. That is the best any client can do; X11 honours both.

package runtime

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	gtkGeomOnce sync.Once
	gtkGeomLib  uintptr

	pGtkWindowGetPosition func(w unsafe.Pointer, x, y *int32)
	pGtkWindowGetSize     func(w unsafe.Pointer, cx, cy *int32)
	pGtkWindowMove        func(w unsafe.Pointer, x, y int32)
	pGtkWindowResize      func(w unsafe.Pointer, cx, cy int32)
	pGtkWindowSetResiz    func(w unsafe.Pointer, resizable bool)
	pGtkWindowSetKeepAbov func(w unsafe.Pointer, setting bool)
	pGtkWindowSetDecor    func(w unsafe.Pointer, setting bool)
	pGtkWindowFullscreen  func(w unsafe.Pointer)
	pGtkWindowUnfullscr   func(w unsafe.Pointer)
)

func loadGTKGeom() {
	gtkGeomOnce.Do(func() {
		for _, name := range []string{"libgtk-3.so.0", "libgtk-3.so"} {
			h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err == nil && h != 0 {
				gtkGeomLib = h
				break
			}
		}
		if gtkGeomLib == 0 {
			return
		}
		reg := func(p any, sym string) {
			defer func() { _ = recover() }() // a missing symbol must not take the process down
			purego.RegisterLibFunc(p, gtkGeomLib, sym)
		}
		reg(&pGtkWindowGetPosition, "gtk_window_get_position")
		reg(&pGtkWindowGetSize, "gtk_window_get_size")
		reg(&pGtkWindowMove, "gtk_window_move")
		reg(&pGtkWindowResize, "gtk_window_resize")
		reg(&pGtkWindowSetResiz, "gtk_window_set_resizable")
		reg(&pGtkWindowSetKeepAbov, "gtk_window_set_keep_above")
		reg(&pGtkWindowSetDecor, "gtk_window_set_decorated")
		reg(&pGtkWindowFullscreen, "gtk_window_fullscreen")
		reg(&pGtkWindowUnfullscr, "gtk_window_unfullscreen")
	})
}

func nativeGetRect(h unsafe.Pointer) (WindowRect, error) {
	loadGTKGeom()
	if pGtkWindowGetPosition == nil || pGtkWindowGetSize == nil {
		return WindowRect{}, fmt.Errorf("goleo: GTK3 geometry symbols unavailable")
	}
	var x, y, cx, cy int32
	pGtkWindowGetPosition(h, &x, &y)
	pGtkWindowGetSize(h, &cx, &cy)
	return WindowRect{X: int(x), Y: int(y), Width: int(cx), Height: int(cy)}, nil
}

func nativeSetRect(h unsafe.Pointer, r WindowRect) error {
	loadGTKGeom()
	if pGtkWindowResize == nil {
		return fmt.Errorf("goleo: GTK3 geometry symbols unavailable")
	}
	pGtkWindowResize(h, int32(r.Width), int32(r.Height))
	if pGtkWindowMove != nil {
		pGtkWindowMove(h, int32(r.X), int32(r.Y)) // no-op under Wayland, by design
	}
	return nil
}

func nativeSetChrome(h unsafe.Pointer, c WindowChrome) error {
	loadGTKGeom()
	if gtkGeomLib == 0 {
		return fmt.Errorf("goleo: GTK3 unavailable")
	}
	if c.Resizable != nil && pGtkWindowSetResiz != nil {
		pGtkWindowSetResiz(h, *c.Resizable)
	}
	if c.AlwaysOnTop != nil && pGtkWindowSetKeepAbov != nil {
		// A hint to the window manager, not a guarantee — some compositors ignore it.
		pGtkWindowSetKeepAbov(h, *c.AlwaysOnTop)
	}
	if c.Decorations != nil && pGtkWindowSetDecor != nil {
		pGtkWindowSetDecor(h, *c.Decorations)
	}
	if c.Fullscreen != nil {
		if *c.Fullscreen && pGtkWindowFullscreen != nil {
			pGtkWindowFullscreen(h)
		} else if !*c.Fullscreen && pGtkWindowUnfullscr != nil {
			pGtkWindowUnfullscr(h)
		}
	}
	return nil
}

func nativeVisibleBounds() (WindowRect, error) {
	// Deliberately unimplemented: the GDK monitor API differs between GTK3 and GTK4 and
	// between X11 and Wayland, and clampToVisible treats ErrUnsupported as "cannot check,
	// trust the stored value" — which is the right behaviour here, since Wayland ignores
	// the position anyway and X11 window managers already keep windows reachable.
	return WindowRect{}, fmt.Errorf("goleo: visible bounds on Linux: %w", errUnsupportedGeom)
}
