// GUI-level verification of the fs-scope confinement (Phase 2) on real hardware.
// A real goleo app with RegisterDesktopFeatures (so RegisterFS is live) and native
// IPC; its embedded page drives the fs plugin over the native channel and asserts
// that in-scope writes succeed while out-of-scope writes and deletes are refused.
// Prints RESULT: PASS and exits 0 when all four hold.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	goleo "github.com/daforester/goleo/runtime"
)

//go:embed all:frontend/dist
var fe embed.FS

func main() {
	tmp, err := os.MkdirTemp("", "fsverify-scope-")
	if err != nil {
		fmt.Println("tempdir:", err)
		os.Exit(1)
	}
	outside, err := os.MkdirTemp("", "fsverify-outside-")
	if err != nil {
		fmt.Println("tempdir:", err)
		os.Exit(1)
	}
	inScope := filepath.Join(tmp, "notes.txt")
	outOfScope := filepath.Join(outside, "victim.txt")
	if werr := os.WriteFile(outOfScope, []byte("ORIGINAL"), 0o644); werr != nil {
		fmt.Println("seed:", werr)
		os.Exit(1)
	}

	app := goleo.New(goleo.Config{
		Title:      "fs-scope verify",
		Width:      520,
		Height:     360,
		WindowMode: goleo.WindowModeWebview,
		NativeIPC:  true,
		EmbedFS:    fe,
		AppID:      "fsverify",
	})
	b := app.Bridge()
	goleo.RegisterBuiltins(b)
	goleo.RegisterDesktopFeatures(b) // includes RegisterFS
	b.AddFSRoot(tmp)                 // the app's allowed working dir

	b.Handle("verify:inScopePath", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return inScope, nil
	})
	b.Handle("verify:outOfScopePath", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return outOfScope, nil
	})

	var once sync.Once
	var failed atomic.Bool
	b.Handle("verify:report", func(ctx context.Context, args json.RawMessage) (any, error) {
		var r struct {
			Native                bool `json:"native"`
			WriteInScopeOK        bool `json:"writeInScopeOK"`
			WriteOutScopeRefused  bool `json:"writeOutScopeRefused"`
			DeleteOutScopeRefused bool `json:"deleteOutScopeRefused"`
			ReadInScopeOK         bool `json:"readInScopeOK"`
		}
		_ = json.Unmarshal(args, &r)

		// The out-of-scope file must be byte-for-byte untouched.
		data, rerr := os.ReadFile(outOfScope)
		survived := rerr == nil && string(data) == "ORIGINAL"

		fmt.Printf("report: native=%v writeIn=%v writeOutRefused=%v deleteOutRefused=%v readIn=%v victimIntact=%v\n",
			r.Native, r.WriteInScopeOK, r.WriteOutScopeRefused, r.DeleteOutScopeRefused, r.ReadInScopeOK, survived)

		ok := r.Native && r.WriteInScopeOK && r.WriteOutScopeRefused && r.DeleteOutScopeRefused && r.ReadInScopeOK && survived
		once.Do(func() {
			if ok {
				fmt.Println("RESULT: PASS (fs confined over native IPC in a real window)")
			} else {
				failed.Store(true)
				fmt.Println("RESULT: FAIL")
			}
			go func() { time.Sleep(300 * time.Millisecond); app.Quit() }()
		})
		return nil, nil
	})

	// Safety net: never hang.
	go func() {
		time.Sleep(35 * time.Second)
		once.Do(func() { failed.Store(true); fmt.Println("RESULT: FAIL (timeout — page never reported)"); app.Quit() })
	}()

	if rerr := app.Run(); rerr != nil {
		fmt.Println("run error:", rerr)
		os.Exit(1)
	}
	// Exit non-zero on a failed verdict. Without this the process printed
	// "RESULT: FAIL" and still exited 0, so the CI step went GREEN on a failing
	// verification — which is exactly the kind of silent pass this spike exists to
	// prevent. It is how the macOS in-scope-write regression reached master.
	if failed.Load() {
		os.Exit(1)
	}
}
