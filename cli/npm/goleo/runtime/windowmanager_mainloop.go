package runtime

import (
	"fmt"
	"sync"
	"time"
)

// mainLoopWindowManager is the in-process window manager for platforms whose GUI
// toolkit is main-thread-only — macOS (AppKit) and Linux (GTK). Unlike the
// Windows inProcWindowManager (one LockOSThread goroutine + Run() per window),
// these cannot host a second run loop on another thread, so a SINGLE run loop on
// the main thread — the primary window's Run() — owns every window.
//
// Additional windows are created by Dispatching NewWebviewWindow onto that
// main-thread loop (glaze.New under the shared NSApplication/GtkApplication;
// never their own Run()), and closed via that window's Destroy (glaze decrements
// its window count and terminates the app only when the last window closes).
// Proven in spikes/glaze-multiwindow. Selected by Config.InProcessWindows on
// darwin/linux.
type mainLoopWindowManager struct {
	app *App

	// primary owns the single main-thread loop; its Dispatch marshals work onto
	// that thread. Registered by runWebview once the window exists.
	primary   *WebviewWindow
	ready     chan struct{}
	readyOnce sync.Once

	mu   sync.Mutex
	next int
	wins map[int]*mainLoopWindow
}

type mainLoopWindow struct {
	win         *WebviewWindow
	exitOnClose bool
	stopPump    func()
}

func newMainLoopWindowManager(app *App) *mainLoopWindowManager {
	return &mainLoopWindowManager{
		app:   app,
		ready: make(chan struct{}),
		wins:  make(map[int]*mainLoopWindow),
	}
}

// setPrimary registers the primary window (main-thread loop owner), unblocking
// any Open calls waiting to create windows on its loop.
func (m *mainLoopWindowManager) setPrimary(win *WebviewWindow) {
	m.mu.Lock()
	m.primary = win
	m.mu.Unlock()
	m.readyOnce.Do(func() { close(m.ready) })
}

func (m *mainLoopWindowManager) Open(opts WindowOptions) (int, error) {
	// Wait for the primary window / main-thread loop to exist.
	select {
	case <-m.ready:
	case <-time.After(10 * time.Second):
		return 0, fmt.Errorf("goleo: OpenWindow timed out waiting for the primary window")
	}

	m.mu.Lock()
	primary := m.primary
	m.next++
	id := m.next
	m.mu.Unlock()

	url, title, width, height := resolveWindowOptions(m.app, opts)

	// GUI object creation is main-thread-only, so build the window inside a
	// Dispatch onto the primary's loop and wait for it. NewWebviewWindow does NOT
	// call Run(); the primary's loop serves this window too.
	done := make(chan *WebviewWindow, 1)
	primary.Dispatch(func() {
		w := NewWebviewWindow(windowConfig{
			Title:    title,
			Width:    width,
			Height:   height,
			Center:   true,
			URL:      url,
			DevTools: m.app.config.DevMode,
			OnInit:   m.app.nativeOnInit(),
		})
		done <- &w
	})

	var win *WebviewWindow
	select {
	case win = <-done:
	case <-time.After(10 * time.Second):
		return 0, fmt.Errorf("goleo: OpenWindow timed out creating the window")
	}
	if win == nil || !win.IsValid() {
		return 0, fmt.Errorf("goleo: failed to create window")
	}

	mw := &mainLoopWindow{win: win, exitOnClose: opts.ExitOnClose}
	// This window shares the app's Bridge (same process), so under Config.NativeIPC
	// it gets its own native-IPC event pump like the primary window.
	if m.app.config.NativeIPC && win.sess != nil {
		mw.stopPump = win.sess.startEventPump()
	}
	m.mu.Lock()
	m.wins[id] = mw
	m.mu.Unlock()

	m.app.Emit("window:opened", map[string]any{"id": id})
	return id, nil
}

func (m *mainLoopWindowManager) Close(id int) error {
	m.mu.Lock()
	mw, ok := m.wins[id]
	if ok {
		delete(m.wins, id)
	}
	primary := m.primary
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("window %d not found", id)
	}

	if mw.stopPump != nil {
		mw.stopPump()
	}
	// Destroy on the main thread; glaze closes just this window and decrements its
	// count (the app keeps running while other windows remain open).
	if primary != nil {
		primary.Dispatch(func() { mw.win.Destroy() })
	}
	m.app.Emit("window:closed", map[string]any{"id": id})
	if mw.exitOnClose {
		m.app.Quit()
	}
	return nil
}

func (m *mainLoopWindowManager) List() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]int, 0, len(m.wins))
	for id := range m.wins {
		ids = append(ids, id)
	}
	return ids
}

func (m *mainLoopWindowManager) CloseAll() {
	// On shutdown, runWebview Terminates the single loop, which closes every
	// window with the app; closing each here is best-effort cleanup for callers
	// that invoke it while the app keeps running.
	for _, id := range m.List() {
		_ = m.Close(id)
	}
}
