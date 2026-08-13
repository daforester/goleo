package runtime

import "testing"

// RememberWindowState is documented to be called from OnStartup, and OnStartup runs inside
// StartServer — BEFORE runWebview creates the window. The first cut restored inline, found
// a nil mainWin, swallowed the error it is documented to swallow, and left every app that
// called it silently starting at Config's default size forever. The restore is therefore
// deferred until the window exists, and this asserts that the deferral is not consumed
// early: nothing may mark the restore done while there is no window to restore onto.
func TestRememberWindowStateDefersUntilTheWindowExists(t *testing.T) {
	app := New(Config{Title: "geom-defer"})

	app.RememberWindowState() // the OnStartup case: no window yet

	if !app.rememberWindowState {
		t.Fatal("RememberWindowState did not arm the save-on-shutdown path")
	}
	if app.windowStateRestored {
		t.Fatal("the restore was marked done with no window — this is exactly the bug: the " +
			"attempt is spent before runWebview creates the window, so geometry never comes back")
	}

	// Once the window exists, runWebview calls this. The window has no native backend in a
	// test, so the restore itself fails; what matters is that the attempt happens now, once.
	app.mainWin = &WebviewWindow{}
	app.restoreSavedWindowState()
	if !app.windowStateRestored {
		t.Error("the deferred restore did not run once mainWin existed")
	}
}

// clampToVisible is the part of window-state restore that is worth testing without a
// window: the geometry maths is platform-independent, and the failure it prevents is the
// one users actually hit — save a window on a second monitor, unplug it, relaunch, and the
// app starts with its window at x=3000 where no display exists. Nothing appears, which
// reads exactly like a crash.
func TestClampToVisibleKeepsWindowsReachable(t *testing.T) {
	// Stand in for the platform call so the test is pure geometry.
	orig := visibleBoundsFn
	t.Cleanup(func() { visibleBoundsFn = orig })
	visibleBoundsFn = func() (WindowRect, error) {
		return WindowRect{X: 0, Y: 0, Width: 1920, Height: 1080}, nil
	}

	cases := []struct {
		name   string
		in     WindowRect
		want   WindowRect
		usable bool
	}{
		{
			name:   "already on screen is untouched",
			in:     WindowRect{X: 100, Y: 100, Width: 800, Height: 600},
			want:   WindowRect{X: 100, Y: 100, Width: 800, Height: 600},
			usable: true,
		},
		{
			name:   "off the right edge comes back",
			in:     WindowRect{X: 3000, Y: 100, Width: 800, Height: 600},
			want:   WindowRect{X: 1120, Y: 100, Width: 800, Height: 600},
			usable: true,
		},
		{
			name:   "off the left edge comes back",
			in:     WindowRect{X: -900, Y: 100, Width: 800, Height: 600},
			want:   WindowRect{X: 0, Y: 100, Width: 800, Height: 600},
			usable: true,
		},
		{
			name:   "below the bottom comes back",
			in:     WindowRect{X: 100, Y: 2000, Width: 800, Height: 600},
			want:   WindowRect{X: 100, Y: 480, Width: 800, Height: 600},
			usable: true,
		},
		{
			// A window saved on a 4K display, restored on a 1080p one.
			name:   "larger than the screen is shrunk to fit",
			in:     WindowRect{X: 0, Y: 0, Width: 3840, Height: 2160},
			want:   WindowRect{X: 0, Y: 0, Width: 1920, Height: 1080},
			usable: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, usable := clampToVisible(c.in)
			if usable != c.usable {
				t.Fatalf("usable = %v, want %v", usable, c.usable)
			}
			if got != c.want {
				t.Errorf("clampToVisible(%+v) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

// When the platform cannot report bounds — Linux, where the GDK monitor API differs across
// GTK3/GTK4 and X11/Wayland — the stored rect must pass through untouched rather than being
// clamped against a zero-sized screen, which would collapse every window to nothing.
func TestClampToVisibleTrustsStoredRectWhenBoundsUnknown(t *testing.T) {
	orig := visibleBoundsFn
	t.Cleanup(func() { visibleBoundsFn = orig })
	visibleBoundsFn = func() (WindowRect, error) { return WindowRect{}, errUnsupportedGeom }

	in := WindowRect{X: 4000, Y: 50, Width: 800, Height: 600}
	got, usable := clampToVisible(in)
	if !usable || got != in {
		t.Errorf("with unknown bounds the rect should pass through: got %+v usable=%v", got, usable)
	}
}
