package runtime

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
)

// Native in-process IPC.
//
// When Config.NativeIPC is set and a native webview hosts the UI, the frontend
// talks to the backend over the webview's own message channel (a bound Go
// function for frontend->backend, evaluated JS for backend->frontend) instead
// of the loopback WebSocket. The wire format is identical to websocket.go's
// {type, data} envelopes, so @goleo/bridge shares one message handler across
// both transports and falls back to WebSocket/HTTP wherever the native channel
// is absent (child-process windows, browser/PWA, mobile).
//
// The HTTP/WebSocket server stays running — it still serves the embedded assets
// in production and remains the transport for those fallback cases. Native IPC
// only replaces the RPC/event surface for the primary in-process window.

// nativeIPCShim is injected via WebviewWindow.Init before the first navigation.
// It advertises the native channel (window.__GOLEO_NATIVE__) and installs an
// inbox that buffers backend->frontend frames until @goleo/bridge registers its
// handler (window.__goleoOnMessage), then drains them.
const nativeIPCShim = `;(function(){
  if (window.__GOLEO_NATIVE__) { return; }
  window.__GOLEO_NATIVE__ = true;
  var queue = [];
  window.__goleoRecv = function(msg){
    if (window.__goleoOnMessage) { window.__goleoOnMessage(msg); }
    else { queue.push(msg); }
  };
  window.__goleoDrain = function(){ var q = queue; queue = []; return q; };
})();`

// nativeOnInit returns the pre-navigation hook that installs the native bridge
// on a window, or nil when NativeIPC is disabled. Wired into the primary
// window's windowConfig.OnInit so the shim and send binding are registered
// before the page loads.
func (a *App) nativeOnInit() func(*WebviewWindow) {
	if a == nil || !a.config.NativeIPC {
		return nil
	}
	return func(win *WebviewWindow) {
		win.Init(nativeIPCShim)
		// __goleoSend is fire-and-forget: the frontend never awaits its promise.
		// Responses arrive asynchronously via window.__goleoRecv, mirroring the
		// WebSocket model where a request's ID correlates its reply.
		if err := win.Bind("__goleoSend", func(payload string) {
			a.onNativeMessage(payload)
		}); err != nil {
			log.Printf("goleo: native IPC bind failed, falling back to WebSocket: %v", err)
		}
	}
}

// onNativeMessage handles one frontend->backend frame. The envelope mirrors
// websocket.go's readPump exactly so both transports share the wire format.
// Runs on the webview UI thread (the Bind callback), so invokes are dispatched
// to their own goroutine — a slow handler must not stall the message loop.
func (a *App) onNativeMessage(payload string) {
	var env struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data,omitempty"`
	}
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		log.Printf("goleo: invalid native message: %v", err)
		return
	}

	switch env.Type {
	case "invoke":
		var req InvokeRequest
		if err := json.Unmarshal(env.Data, &req); err != nil {
			log.Printf("goleo: invalid native invoke: %v", err)
			return
		}
		go func(req InvokeRequest) {
			resp := a.bridge.HandleRequest(req)
			a.nativePush(map[string]any{"type": "invokeResult", "data": resp})
		}(req)

	case "event":
		var msg EventMessage
		if err := json.Unmarshal(env.Data, &msg); err != nil {
			log.Printf("goleo: invalid native event: %v", err)
			return
		}
		a.bridge.DispatchEvent(msg.Event, msg.Data)

	case "ping":
		a.nativePush(map[string]string{"type": "pong"})
	}
}

// nativePush delivers a backend->frontend envelope over the native channel.
func (a *App) nativePush(env any) {
	data, err := json.Marshal(env)
	if err != nil {
		return
	}
	a.nativeEvalRecv(string(data))
}

// nativeEvalRecv evaluates window.__goleoRecv(<json>) on the window's UI thread.
// The nativeEvalFn hook overrides the real Dispatch+Eval in tests. Guards
// against a window that has been (or is being) destroyed so a late event push
// during shutdown is a no-op rather than a use-after-free.
func (a *App) nativeEvalRecv(jsonArg string) {
	if a.nativeEvalFn != nil {
		a.nativeEvalFn(jsonArg)
		return
	}
	win := a.getNativeWin()
	if win == nil || !win.IsValid() {
		return
	}
	js := "window.__goleoRecv(" + jsSafeJSON(jsonArg) + ");"
	win.Dispatch(func() {
		if w := a.getNativeWin(); w != nil && w.IsValid() {
			w.Eval(js)
		}
	})
}

// startNativeEventPump forwards bridge events to the native window until the
// returned stop func runs or the app context is cancelled — the native
// equivalent of server.handleEvents broadcasting to WebSocket clients.
func (a *App) startNativeEventPump(win *WebviewWindow) func() {
	a.setNativeWin(win)
	ch := a.bridge.Subscribe()
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-done:
				return
			case <-a.ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				a.nativePush(map[string]any{"type": "event", "data": msg})
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			a.bridge.Unsubscribe(ch)
			a.setNativeWin(nil)
		})
	}
}

func (a *App) setNativeWin(win *WebviewWindow) {
	a.nativeMu.Lock()
	a.nativeWin = win
	a.nativeMu.Unlock()
}

func (a *App) getNativeWin() *WebviewWindow {
	a.nativeMu.Lock()
	defer a.nativeMu.Unlock()
	return a.nativeWin
}

// jsSafeJSON escapes U+2028 (line separator) and U+2029 (paragraph
// separator): both are valid inside JSON strings but were not valid in
// JavaScript string literals before ES2019, so escaping them keeps the JSON
// safe to splice into an evaluated expression on any webview engine. All
// three code points are built from their values to keep this file ASCII-only.
func jsSafeJSON(s string) string {
	ls := string(rune(0x2028))
	ps := string(rune(0x2029))
	if !strings.Contains(s, ls) && !strings.Contains(s, ps) {
		return s
	}
	esc := string(rune(0x5c))
	s = strings.ReplaceAll(s, ls, esc+"u2028")
	s = strings.ReplaceAll(s, ps, esc+"u2029")
	return s
}
