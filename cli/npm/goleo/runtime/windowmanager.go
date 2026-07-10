package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
)

// windowSpawner is the common contract for opening/closing additional windows,
// implemented by both the multi-process WindowManager (default, cross-platform)
// and the in-process manager (Windows, opt-in via Config.InProcessWindows).
type windowSpawner interface {
	Open(opts WindowOptions) (int, error)
	Close(id int) error
	List() []int
	CloseAll()
}

// WindowOptions describes an additional window to open at runtime.
type WindowOptions struct {
	Title  string `json:"title"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// URL, if set, is loaded verbatim. Otherwise the window loads the app's own
	// server root plus Path (e.g. Path "/settings" → "<serverURL>/settings").
	URL  string `json:"url"`
	Path string `json:"path"`
	// ExitOnClose quits the whole app when this window closes (via App.Quit).
	// Default false: closing just closes the window; the app keeps running.
	ExitOnClose bool `json:"exitOnClose"`
}

// resolveWindowOptions fills in defaults from the app config.
func resolveWindowOptions(app *App, opts WindowOptions) (url, title string, width, height int) {
	url = opts.URL
	if url == "" {
		url = app.serverURL(app.port) + opts.Path
	}
	title = opts.Title
	if title == "" {
		title = app.config.Title
	}
	width = opts.Width
	if width == 0 {
		width = app.config.Width
	}
	height = opts.Height
	if height == 0 {
		height = app.config.Height
	}
	return
}

// --- Multi-process window manager (default, cross-platform) ---

type procWindow struct {
	cmd         *exec.Cmd
	exitOnClose bool
}

// WindowManager tracks additional webview windows, each running as a child
// process of this executable (see window_child.go). The primary window is still
// hosted in-process by App.runWebview; this manages every window opened after
// startup via App.OpenWindow / the goleo:window* bridge commands.
type WindowManager struct {
	app  *App
	mu   sync.Mutex
	next int
	wins map[int]*procWindow
}

func newWindowManager(app *App) *WindowManager {
	return &WindowManager{app: app, wins: make(map[int]*procWindow)}
}

// Open spawns a new window process and returns its id. The child connects to
// this process's server as an ordinary bridge client, so cross-window state and
// events flow through the existing hub.
func (wm *WindowManager) Open(opts WindowOptions) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("locate executable: %w", err)
	}
	url, title, width, height := resolveWindowOptions(wm.app, opts)

	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		envWindowChild+"=1",
		envWindowURL+"="+url,
		envWindowTitle+"="+title,
		envWindowWidth+"="+strconv.Itoa(width),
		envWindowHeight+"="+strconv.Itoa(height),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start window process: %w", err)
	}

	wm.mu.Lock()
	wm.next++
	id := wm.next
	wm.wins[id] = &procWindow{cmd: cmd, exitOnClose: opts.ExitOnClose}
	wm.mu.Unlock()

	// Reap the process when its window closes, notify the frontend, and quit
	// the app if this window was flagged ExitOnClose.
	go func() {
		cmd.Wait()
		wm.mu.Lock()
		delete(wm.wins, id)
		wm.mu.Unlock()
		wm.app.Emit("window:closed", map[string]any{"id": id})
		if opts.ExitOnClose {
			wm.app.Quit()
		}
	}()

	wm.app.Emit("window:opened", map[string]any{"id": id})
	return id, nil
}

// Close terminates the window with the given id. The webview child holds no
// unsaved state (it is pure UI), so killing the process is safe.
func (wm *WindowManager) Close(id int) error {
	wm.mu.Lock()
	pw, ok := wm.wins[id]
	wm.mu.Unlock()
	if !ok {
		return fmt.Errorf("window %d not found", id)
	}
	if pw.cmd.Process != nil {
		return pw.cmd.Process.Kill()
	}
	return nil
}

// List returns the ids of all currently open managed windows.
func (wm *WindowManager) List() []int {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	ids := make([]int, 0, len(wm.wins))
	for id := range wm.wins {
		ids = append(ids, id)
	}
	return ids
}

// CloseAll terminates every managed window; called during shutdown.
func (wm *WindowManager) CloseAll() {
	wm.mu.Lock()
	cmds := make([]*exec.Cmd, 0, len(wm.wins))
	for _, pw := range wm.wins {
		cmds = append(cmds, pw.cmd)
	}
	wm.mu.Unlock()
	for _, cmd := range cmds {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}
}

// --- In-process window manager (Windows; opt-in) ---

type inprocWindow struct {
	win         *WebviewWindow
	exitOnClose bool
}

// inProcWindowManager hosts each additional window in-process, on its own
// locked OS thread, instead of a child process (proven on Windows — see
// spikes/win-multiwindow). Lower memory + the path toward native-bind IPC and
// an in-process tray. Selected when Config.InProcessWindows is set on Windows.
type inProcWindowManager struct {
	app  *App
	mu   sync.Mutex
	next int
	wins map[int]*inprocWindow
}

func newInProcWindowManager(app *App) *inProcWindowManager {
	return &inProcWindowManager{app: app, wins: make(map[int]*inprocWindow)}
}

func (m *inProcWindowManager) Open(opts WindowOptions) (int, error) {
	url, title, width, height := resolveWindowOptions(m.app, opts)

	m.mu.Lock()
	m.next++
	id := m.next
	m.mu.Unlock()

	// Each window owns a locked OS thread so its message loop is independent
	// (Windows delivers messages per-thread). ready hands the window back so
	// Close can Dispatch to its thread.
	ready := make(chan *WebviewWindow, 1)
	go func() {
		runtime.LockOSThread()
		w := NewWebviewWindow(windowConfig{
			Title:    title,
			Width:    width,
			Height:   height,
			Center:   true,
			URL:      url,
			DevTools: m.app.config.DevMode,
		})
		ready <- &w
		w.Run() // blocks this thread until the window closes
		w.Destroy()
		m.mu.Lock()
		delete(m.wins, id)
		m.mu.Unlock()
		m.app.Emit("window:closed", map[string]any{"id": id})
		if opts.ExitOnClose {
			m.app.Quit()
		}
	}()

	win := <-ready
	m.mu.Lock()
	m.wins[id] = &inprocWindow{win: win, exitOnClose: opts.ExitOnClose}
	m.mu.Unlock()
	m.app.Emit("window:opened", map[string]any{"id": id})
	return id, nil
}

func (m *inProcWindowManager) Close(id int) error {
	m.mu.Lock()
	iw, ok := m.wins[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("window %d not found", id)
	}
	// Terminate must run on the window's own UI thread.
	iw.win.Dispatch(func() { iw.win.Terminate() })
	return nil
}

func (m *inProcWindowManager) List() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]int, 0, len(m.wins))
	for id := range m.wins {
		ids = append(ids, id)
	}
	return ids
}

func (m *inProcWindowManager) CloseAll() {
	for _, id := range m.List() {
		m.Close(id)
	}
}
