// Runtime-level verification of the goleo desktop stack on Linux/macOS — the
// REAL integration, not the raw-glaze spikes. One app exercises three things the
// spikes couldn't, all cgo-free (glaze default backend):
//
//  1. Native IPC (Config.NativeIPC)          — the page talks to the Bridge over
//     the in-process channel (native flag).
//  2. WebKitGTK permission auto-grant shim    — getUserMedia gets PAST the prompt
//     (runtime/webview_glaze_permissions_*)     instead of hanging the GTK loop.
//  3. mainLoopWindowManager (InProcessWindows) — a 2nd window opened via
//     App.OpenWindow on the single loop.
//
// Serves its embedded UI over http://127.0.0.1 (secure context, so getUserMedia
// is available). Prints RESULT: PASS + exits 0 when all three are confirmed. Run
// under xvfb on Linux via scripts/verify-linux-docker.* / glaze-verify.yml.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	goleo "github.com/daforester/goleo/runtime"
)

//go:embed all:frontend/dist
var fe embed.FS

func main() {
	var (
		mu          sync.Mutex
		nativeWin1  bool // window 1 reached the Bridge over the native channel
		permGranted bool // getUserMedia got past the permission gate (shim fired)
		win2Ready   bool // 2nd window (OpenWindow -> mainLoopWindowManager) loaded
		doneOnce    sync.Once
	)

	var app *goleo.App
	app = goleo.New(goleo.Config{
		Title:            "rt-verify",
		Width:            420,
		Height:           300,
		WindowMode:       goleo.WindowModeWebview,
		NativeIPC:        true, // exercise native IPC
		InProcessWindows: true, // -> mainLoopWindowManager on macOS/Linux
		EmbedFS:          fe,
		OnReady: func(ctx context.Context) {
			// Open a 2nd in-process window on the single main-thread loop.
			if _, err := app.OpenWindow(goleo.WindowOptions{Path: "/?win=2", Width: 420, Height: 300}); err != nil {
				fmt.Println("OpenWindow error:", err)
			}
		},
	})
	goleo.RegisterBuiltins(app.Bridge())

	finish := func() {
		mu.Lock()
		ok := nativeWin1 && permGranted && win2Ready
		mu.Unlock()
		if ok {
			doneOnce.Do(func() {
				fmt.Println("RESULT: PASS (native IPC + permission auto-grant + in-process 2nd window on Linux)")
				go func() { time.Sleep(300 * time.Millisecond); app.Quit() }()
			})
		}
	}

	app.Bridge().Handle("smoke:report", func(ctx context.Context, args json.RawMessage) (any, error) {
		fmt.Println("report:", string(args))
		var m struct {
			Phase, Win, Verdict string
			Native              bool
		}
		_ = json.Unmarshal(args, &m)
		mu.Lock()
		switch {
		case m.Phase == "ready" && m.Win == "1" && m.Native:
			nativeWin1 = true
		case m.Phase == "ready" && m.Win == "2":
			win2Ready = true
		case m.Phase == "perm" && m.Win == "1":
			// Any verdict except an outright denial means the prompt was answered
			// (the shim granted); NotFound/Overconstrained on a camera-less box
			// still proves that. Only NotAllowed/Security = denied.
			if m.Verdict != "NotAllowedError" && m.Verdict != "SecurityError" && m.Verdict != "" {
				permGranted = true
			}
		}
		mu.Unlock()
		finish()
		return map[string]any{"ok": true}, nil
	})

	// Safety net: never hang the run. A hang here (e.g. if the permission shim
	// failed and getUserMedia wedged the GTK loop) is itself a failure — the
	// docker `timeout` wrapper will hard-kill it, and RESULT: PASS won't print.
	go func() {
		time.Sleep(40 * time.Second)
		mu.Lock()
		fmt.Printf("RESULT: FAIL (native1=%v permGranted=%v win2=%v)\n", nativeWin1, permGranted, win2Ready)
		mu.Unlock()
		app.Quit()
	}()

	if err := app.Run(); err != nil {
		fmt.Println("run error:", err)
		os.Exit(1)
	}
}
