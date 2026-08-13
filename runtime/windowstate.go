package runtime

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/daforester/goleo/runtime/store"
)

// T3 — window state persistence: remember a window's size and position across launches.
//
// Users notice its absence immediately and never mention it when it works, which is the
// whole argument for having it.
//
// Stored through runtime/store, so it lands in the app's own data directory alongside
// everything else and inherits the per-app separation that 0.9.0 introduced.

const windowStateKey = "goleo.windowState"

// SaveWindowState records the primary window's current geometry.
//
// Errors are returned rather than logged: a caller wiring this to a shutdown hook can
// decide whether a failure is worth mentioning, and on a platform with no geometry support
// it is not — the error wraps errors.ErrUnsupported so that case is distinguishable.
func (a *App) SaveWindowState() error {
	win := a.mainWin
	if win == nil {
		return errNoWindow
	}
	r, err := win.Rect()
	if err != nil {
		return err
	}
	// A minimised or hidden window reports a degenerate or off-screen rect on some
	// platforms (Windows uses -32000 for a minimised window's position). Persisting that
	// would restore an invisible window next launch, so refuse rather than store it.
	if r.Width <= 0 || r.Height <= 0 || r.X < -30000 || r.Y < -30000 {
		return fmt.Errorf("goleo: refusing to save a minimised or degenerate window rect %+v", r)
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	st, err := store.Default()
	if err != nil {
		return err
	}
	return st.Set(windowStateKey, json.RawMessage(b))
}

// RestoreWindowState applies the saved geometry to the primary window.
//
// Returns (false, nil) when there is nothing saved — the common first-run case, which is
// not an error and should not be logged as one.
func (a *App) RestoreWindowState() (bool, error) {
	win := a.mainWin
	if win == nil {
		return false, errNoWindow
	}
	st, err := store.Default()
	if err != nil {
		return false, err
	}
	raw, ok := st.Get(windowStateKey)
	if !ok {
		return false, nil
	}
	var r WindowRect
	if err := json.Unmarshal(raw, &r); err != nil {
		// Corrupt or from an older schema: drop it rather than failing the launch.
		return false, nil
	}
	if r.Width <= 0 || r.Height <= 0 {
		return false, nil
	}

	clamped, usable := clampToVisible(r)
	if !usable {
		// The saved monitor is gone and nothing sensible survives clamping. Falling back to
		// Config's defaults is right: a window the user cannot see is worse than one in the
		// wrong place, and this is exactly the unplugged-second-monitor case.
		return false, nil
	}
	if err := win.SetRect(clamped); err != nil {
		if errors.Is(err, errors.ErrUnsupported) {
			return false, nil // platform cannot restore; not worth surfacing
		}
		return false, err
	}
	return true, nil
}

// RememberWindowState restores the saved geometry and arranges to save it on shutdown.
//
// Call it from OnStartup:
//
//	OnStartup: func(ctx context.Context) { a.RememberWindowState() }
//
// THE WINDOW DOES NOT EXIST YET AT THAT POINT, which is why the restore is deferred
// rather than done here. OnStartup runs inside StartServer, and the primary window is
// created afterwards in runWebview; the first cut restored inline, found a nil window,
// swallowed the error it is documented to swallow, and left every app that called this
// silently starting at Config's default size forever. runWebview applies the restore as
// soon as the window is there (restoreSavedWindowState).
//
// Calling it later — from OnReady, or after OpenWindow — still works: the window exists
// by then and the restore happens immediately.
//
// Errors are swallowed on purpose. Every failure mode here is benign — no saved state on
// first run, a platform with no geometry support, a monitor that has gone away — and none
// of them is a reason to interrupt startup or shutdown. Call SaveWindowState or
// RestoreWindowState directly if you want to see them.
func (a *App) RememberWindowState() {
	a.rememberWindowState = true
	a.restoreSavedWindowState()
}

// restoreSavedWindowState applies the saved geometry once, if it was asked for and the
// window exists. Called both by RememberWindowState (late callers) and by runWebview
// (the OnStartup case, where the window is created after the call). The once-only flag
// is what makes calling it from both safe.
func (a *App) restoreSavedWindowState() {
	if !a.rememberWindowState || a.windowStateRestored || a.mainWin == nil {
		return
	}
	a.windowStateRestored = true
	_, _ = a.RestoreWindowState()
}
