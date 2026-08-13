package runtime

import (
	"encoding/json"
	"os"
	"strconv"
)

// Multi-window model.
//
// Native OS webviews (WebView2, WKWebView, WebKitGTK) are single-window and own
// the GUI thread's message loop, so a second window cannot simply be created on
// another goroutine. Goleo already runs a loopback HTTP/WebSocket server that
// every window connects to as a client, so extra windows are spawned as *child
// processes* of the same executable — each hosts one webview pointed at the
// shared backend. This keeps the code cgo-free (each child uses the normal
// per-OS WebviewWindow backend) and reuses the existing bridge hub for
// cross-window IPC. The main process stays the single backend/controller.
//
// A child process is the same binary re-executed with GOLEO_WINDOW=1 plus the
// window parameters below. App.Run detects this and runs runWindowChild instead
// of starting the server.

const (
	envWindowChild  = "GOLEO_WINDOW"
	envWindowURL    = "GOLEO_WINDOW_URL"
	envWindowTitle  = "GOLEO_WINDOW_TITLE"
	envWindowWidth  = "GOLEO_WINDOW_WIDTH"
	envWindowHeight = "GOLEO_WINDOW_HEIGHT"
	// envWindowChrome carries WindowChrome as JSON. One variable rather than four,
	// because each field is tri-state (unset / true / false) and a JSON object with
	// omitempty already encodes exactly that, while "" versus "false" in an env var
	// does not.
	envWindowChrome = "GOLEO_WINDOW_CHROME"
)

// encodeChromeEnv renders chrome for envWindowChrome, returning "" when there is
// nothing to say so the parent does not set an empty variable.
func encodeChromeEnv(c WindowChrome) (string, error) {
	if c.isZero() {
		return "", nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// decodeChromeEnv parses envWindowChrome. A malformed value yields the zero chrome —
// the parent wrote it, so the failure is a goleo bug rather than user input, and a
// window with default decorations is a better outcome than a child that refuses to
// start.
func decodeChromeEnv(s string) WindowChrome {
	if s == "" {
		return WindowChrome{}
	}
	var c WindowChrome
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return WindowChrome{}
	}
	return c
}

// isWindowChild reports whether this process was spawned to host a single
// webview window (see WindowManager.Open).
func isWindowChild() bool {
	return os.Getenv(envWindowChild) == "1"
}

// runWindowChild creates one webview for the URL passed by the parent and
// blocks on the GUI loop until the window is closed, then returns so the
// process exits. It must run on the main goroutine (App.Run is called from
// main), which the webview backends require for GUI-thread affinity.
func (a *App) runWindowChild() error {
	cfg := windowConfig{
		Title:    envOrDefault(envWindowTitle, a.config.Title),
		Width:    envIntOrDefault(envWindowWidth, a.config.Width),
		Height:   envIntOrDefault(envWindowHeight, a.config.Height),
		Center:   true,
		URL:      os.Getenv(envWindowURL),
		DevTools: a.config.DevMode,
		Chrome:   decodeChromeEnv(os.Getenv(envWindowChrome)),
	}

	win := NewWebviewWindow(cfg)
	win.Run() // blocks until the window closes
	win.Destroy()
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
