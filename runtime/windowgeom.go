package runtime

import (
	"errors"
	"fmt"
)

// Native window geometry and chrome, for Track T tier 1 (T2 window chrome, T3 window
// state persistence).
//
// WHY THIS EXISTS RATHER THAN CALLING THE WEBVIEW BINDING: glaze exposes SetSize and
// SetTitle and nothing else — no geometry getters, no SetPosition. You cannot save a
// window position you cannot read. What it does expose is the native window handle
// (WebviewWindow.NativeHandle: HWND / NSWindow / GtkWindow*), and runtime/menu_windows.go
// already establishes the pattern of taking that handle and driving the OS directly
// through purego. This is the same move, generalised.
//
// Each platform file implements the four primitives below. Unsupported platforms return
// errors.ErrUnsupported so callers can branch on it, which is the convention every other
// goleo feature follows.

// WindowRect is a window's outer frame in screen coordinates, top-left origin.
//
// Top-left origin on EVERY platform, including macOS, whose native NSWindow frame uses a
// bottom-left origin with Y increasing upward. The darwin implementation converts, because
// a geometry value that means different things per platform is a bug generator — the whole
// point here is that a saved rect can be restored on the machine that saved it without the
// caller knowing which OS it came from.
type WindowRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// WindowChrome is the set of window decorations and behaviours T2 covers. A nil field
// means "leave as-is", so a caller can change one property without restating the others.
type WindowChrome struct {
	Resizable   *bool `json:"resizable,omitempty"`
	AlwaysOnTop *bool `json:"alwaysOnTop,omitempty"`
	Fullscreen  *bool `json:"fullscreen,omitempty"`
	Decorations *bool `json:"decorations,omitempty"`
}

var errNoWindow = errors.New("goleo: no native window (browser or mobile mode)")

// errUnsupportedGeom lets a platform file report "this part is not implemented here"
// while still wrapping errors.ErrUnsupported, which is what callers branch on.
var errUnsupportedGeom = errors.ErrUnsupported

// MainWindow returns the primary native window, or nil when there is none — browser mode,
// mobile, or before the window has been created.
//
// Needed because Rect/SetRect/SetChrome hang off *WebviewWindow, which nothing else
// exposed: T2's chrome API would have been public and unreachable without this.
func (a *App) MainWindow() *WebviewWindow { return a.mainWin }

// Rect reports the window's outer frame in screen coordinates.
func (win *WebviewWindow) Rect() (WindowRect, error) {
	h := win.NativeHandle()
	if h == nil {
		return WindowRect{}, errNoWindow
	}
	return nativeGetRect(h)
}

// SetRect moves and resizes the window.
func (win *WebviewWindow) SetRect(r WindowRect) error {
	h := win.NativeHandle()
	if h == nil {
		return errNoWindow
	}
	if r.Width <= 0 || r.Height <= 0 {
		return fmt.Errorf("goleo: window size must be positive, got %dx%d", r.Width, r.Height)
	}
	return nativeSetRect(h, r)
}

// SetChrome applies decoration and behaviour changes. Fields left nil are untouched.
func (win *WebviewWindow) SetChrome(c WindowChrome) error {
	h := win.NativeHandle()
	if h == nil {
		return errNoWindow
	}
	return nativeSetChrome(h, c)
}

// VisibleBounds is the union of every display's work area, used to clamp a restored
// window back on-screen. Returns ErrUnsupported where the platform code cannot report it,
// in which case clampToVisible passes the rect through unchanged.
func VisibleBounds() (WindowRect, error) { return visibleBoundsFn() }

// visibleBoundsFn is a variable so tests can substitute a known screen and exercise the
// clamping maths without a display. Production always uses the platform implementation.
var visibleBoundsFn = nativeVisibleBounds

// clampToVisible keeps a restored rect reachable.
//
// The case that matters is not exotic: save a window on a second monitor, unplug it, relaunch.
// The stored position is now off-screen, and on Windows a window at x=3000 with no display
// there is invisible and unreachable — the user sees the app start and no window appear,
// which reads exactly like a crash. So a restored rect is always clamped, and a rect that
// cannot be made to intersect the visible area is discarded entirely in favour of the
// caller's default rather than being nudged to a corner.
func clampToVisible(r WindowRect) (WindowRect, bool) {
	b, err := visibleBoundsFn()
	if err != nil || b.Width <= 0 || b.Height <= 0 {
		return r, true // cannot check; trust the stored value
	}

	// A window is reachable if a usable strip of its title bar is inside the work area.
	const minVisible = 80
	if r.Width > b.Width {
		r.Width = b.Width
	}
	if r.Height > b.Height {
		r.Height = b.Height
	}
	if r.X+r.Width < b.X+minVisible {
		r.X = b.X
	}
	if r.Y+r.Height < b.Y+minVisible {
		r.Y = b.Y
	}
	if r.X > b.X+b.Width-minVisible {
		r.X = b.X + b.Width - r.Width
	}
	if r.Y > b.Y+b.Height-minVisible {
		r.Y = b.Y + b.Height - r.Height
	}
	if r.Width < 1 || r.Height < 1 {
		return r, false
	}
	return r, true
}
