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
// Each natively-bridged window owns a nativeSession — the primary window and,
// under Config.InProcessWindows, each in-process window. The HTTP/WebSocket
// server stays running: it still serves the embedded assets in production and
// remains the transport for the fallback cases above.

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

// nativeEvaler is the subset of a per-platform webview backend the native bridge
// needs to push frames to the frontend. The glaze backend (and the cgo webview_go
// fallback) satisfy it; WebviewWindow.evaler() adapts to it (nil on the mobile stub).
type nativeEvaler interface {
	Dispatch(func())
	Eval(string)
}

// nativeSession is one window's native IPC channel: it decodes frontend frames
// into the shared Bridge and pushes results/events back over that window's
// webview. Guarded so a push after the window closes is a no-op, not a
// use-after-free.
type nativeSession struct {
	app    *App
	ev     nativeEvaler
	evalFn func(string) // test hook; when set, replaces the real Dispatch+Eval
	mu     sync.Mutex
	alive  bool
}

func (a *App) newNativeSession(ev nativeEvaler) *nativeSession {
	return &nativeSession{app: a, ev: ev, alive: true}
}

// nativeOnInit returns the pre-navigation hook that installs a native session on
// a window, or nil when NativeIPC is disabled. Wired through windowConfig.OnInit
// so the shim and send binding register before the first navigation; the session
// is stored on the window (win.sess) for the caller to drive its event pump.
func (a *App) nativeOnInit() func(*WebviewWindow) {
	if a == nil || !a.config.NativeIPC {
		return nil
	}
	return func(win *WebviewWindow) {
		s := a.newNativeSession(win.evaler())
		win.sess = s
		win.Init(nativeIPCShim)
		// __goleoSend is fire-and-forget: the frontend never awaits its promise.
		// Responses arrive asynchronously via window.__goleoRecv, mirroring the
		// WebSocket model where a request's ID correlates its reply.
		if err := win.Bind("__goleoSend", func(payload string) {
			s.onMessage(payload)
		}); err != nil {
			log.Printf("goleo: native IPC bind failed, falling back to WebSocket: %v", err)
		}
	}
}

// onMessage handles one frontend->backend frame. The envelope mirrors
// websocket.go's readPump exactly so both transports share the wire format.
// Runs on the webview UI thread (the Bind callback), so invokes are dispatched
// to their own goroutine — a slow handler must not stall the message loop.
func (s *nativeSession) onMessage(payload string) {
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
			resp := s.app.bridge.HandleRequest(req)
			s.push(map[string]any{"type": "invokeResult", "data": resp})
		}(req)

	case "event":
		var msg EventMessage
		if err := json.Unmarshal(env.Data, &msg); err != nil {
			log.Printf("goleo: invalid native event: %v", err)
			return
		}
		s.app.bridge.DispatchEvent(msg.Event, msg.Data)

	case "ping":
		s.push(map[string]string{"type": "pong"})
	}
}

// push delivers a backend->frontend envelope over this session's channel.
func (s *nativeSession) push(env any) {
	data, err := json.Marshal(env)
	if err != nil {
		return
	}
	s.evalRecv(string(data))
}

// evalRecv evaluates window.__goleoRecv(<json>) on the window's UI thread. The
// evalFn hook overrides the real Dispatch+Eval in tests. Guards against a
// window that has been (or is being) closed.
func (s *nativeSession) evalRecv(jsonArg string) {
	s.mu.Lock()
	fn, ev, alive := s.evalFn, s.ev, s.alive
	s.mu.Unlock()

	if fn != nil {
		fn(jsonArg)
		return
	}
	if !alive || ev == nil {
		return
	}
	js := "window.__goleoRecv(" + jsSafeJSON(jsonArg) + ");"
	ev.Dispatch(func() {
		s.mu.Lock()
		ok := s.alive
		s.mu.Unlock()
		if ok {
			ev.Eval(js)
		}
	})
}

// startEventPump forwards bridge events to this session's window until the
// returned stop func runs or the app context is cancelled — the native
// equivalent of server.handleEvents broadcasting to WebSocket clients. stop also
// marks the session closed so no further frames are pushed to a dead window.
func (s *nativeSession) startEventPump() func() {
	ch := s.app.bridge.Subscribe()
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-done:
				return
			case <-s.app.ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				s.push(map[string]any{"type": "event", "data": msg})
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			s.app.bridge.Unsubscribe(ch)
			s.mu.Lock()
			s.alive = false
			s.mu.Unlock()
		})
	}
}

// jsSafeJSON escapes U+2028 (line separator) and U+2029 (paragraph separator):
// both are valid inside JSON strings but were not valid in JavaScript string
// literals before ES2019, so escaping them keeps the JSON safe to splice into an
// evaluated expression on any webview engine. The code points are built from
// their values to keep this file ASCII-only.
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
