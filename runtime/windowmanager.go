package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

// WindowOptions describes an additional window to open at runtime.
type WindowOptions struct {
	Title  string `json:"title"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// URL, if set, is loaded verbatim. Otherwise the window loads the app's own
	// server root plus Path (e.g. Path "/settings" → "<serverURL>/settings").
	URL  string `json:"url"`
	Path string `json:"path"`
}

// WindowManager tracks additional webview windows, each running as a child
// process of this executable (see window_child.go). The primary window is still
// hosted in-process by App.runWebview; this manages every window opened after
// startup via App.OpenWindow / the goleo:window* bridge commands.
type WindowManager struct {
	app  *App
	mu   sync.Mutex
	next int
	wins map[int]*exec.Cmd
}

func newWindowManager(app *App) *WindowManager {
	return &WindowManager{app: app, wins: make(map[int]*exec.Cmd)}
}

// Open spawns a new window process and returns its id. The child connects to
// this process's server as an ordinary bridge client, so cross-window state and
// events flow through the existing hub.
func (wm *WindowManager) Open(opts WindowOptions) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("locate executable: %w", err)
	}

	url := opts.URL
	if url == "" {
		url = wm.app.serverURL(wm.app.port) + opts.Path
	}
	title := opts.Title
	if title == "" {
		title = wm.app.config.Title
	}
	width := opts.Width
	if width == 0 {
		width = wm.app.config.Width
	}
	height := opts.Height
	if height == 0 {
		height = wm.app.config.Height
	}

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
	wm.wins[id] = cmd
	wm.mu.Unlock()

	// Reap the process when its window closes and notify the frontend.
	go func() {
		cmd.Wait()
		wm.mu.Lock()
		delete(wm.wins, id)
		wm.mu.Unlock()
		wm.app.Emit("window:closed", map[string]any{"id": id})
	}()

	wm.app.Emit("window:opened", map[string]any{"id": id})
	return id, nil
}

// Close terminates the window with the given id. The webview child holds no
// unsaved state (it is pure UI), so killing the process is safe.
func (wm *WindowManager) Close(id int) error {
	wm.mu.Lock()
	cmd, ok := wm.wins[id]
	wm.mu.Unlock()
	if !ok {
		return fmt.Errorf("window %d not found", id)
	}
	if cmd.Process != nil {
		return cmd.Process.Kill()
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
	for _, cmd := range wm.wins {
		cmds = append(cmds, cmd)
	}
	wm.mu.Unlock()
	for _, cmd := range cmds {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}
}
