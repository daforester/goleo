// End-to-end verification of the REAL @goleo/bridge npm package running inside a
// REAL webview against a REAL Go backend.
//
// Why this exists. The TypeScript suite (bridge/src/*.test.ts, 55 tests) drives the
// bridge against a FakeSocket, and the Go suite (runtime/ws_e2e_test.go,
// nativeipc_test.go) drives the wire format from hand-written frames. Both are
// green while neither has ever executed the two halves TOGETHER. Three things are
// therefore completely unverified:
//
//  1. The NATIVE transport selection. bridge.ts prefers window.__GOLEO_NATIVE__ and
//     falls back to WebSocket; every TS test stubs global WebSocket, so it always
//     takes the fallback branch. The native branch — the DEFAULT for a desktop
//     window with Config.NativeIPC — has no coverage at all.
//  2. That the base64 binary encoding agrees. Go writes base64.StdEncoding; fs.ts
//     decodes with its own chunked base64ToBytes/bytesToBase64. The TS tests assert
//     what fs.ts produces, the Go tests assert what Go accepts, and nobody checks
//     they are the same alphabet, padding and chunking. This was broken in both
//     directions before Phase 3 — the class of bug that survives two green suites.
//  3. That a backend error reaches the page as its own text. fs.ts rethrows the
//     real error when connected, instead of masking it as "requires the Go backend".
//     That fix shipped unexecuted.
//
// So: SchemeAssets + NativeIPC (no TCP port at all), the page loads the built
// dist/ of @goleo/bridge as ES modules, and reports what actually happened.
// Prints RESULT: PASS and exits 0 only when every check passes.
//
// Run: node prepare.mjs && ./bridge-e2e-verify
// prepare.mjs does the go build itself, because the page is embedded into the
// binary — copying new JS without recompiling verifies the PREVIOUS page.
// CI:  .github/workflows/glaze-verify.yml (macos-14, ubuntu xvfb, windows)
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	goleo "github.com/daforester/goleo/runtime"
)

//go:embed all:frontend/dist
var fe embed.FS

// The exact bytes the page round-trips. Deliberately NOT valid UTF-8: 0x80-0xFF
// and an embedded NUL are what the pre-Phase-3 map[string]string return value and
// TextDecoder write path silently mangled. Valid ASCII would pass either way and
// prove nothing.
var binaryPayload = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // a real PNG magic
	0x00, 0x01, 0xFE, 0xFF, 0x80, 0x7F, 0xC0, 0xC1,
	0xF0, 0x9F, 0x92, 0xA9, // a valid 4-byte sequence, to catch over-eager decoding
	0xED, 0xA0, 0x80, // a lone surrogate half: invalid UTF-8, replaced by U+FFFD if decoded as text
}

type report struct {
	Native      bool     `json:"native"`
	Origin      string   `json:"origin"`
	BinaryOK    bool     `json:"binaryOK"`
	BinaryHex   string   `json:"binaryHex"`
	DeniedOK    bool     `json:"deniedOK"`
	DeniedError string   `json:"deniedError"`
	AppDataOK   bool     `json:"appDataOK"`
	Failures    []string `json:"failures"`
}

func main() {
	// Where the page will be told to attempt a write it must NOT be allowed to make.
	// Handed over by the backend so the spike stays platform-agnostic.
	deniedPath := filepath.Join(os.TempDir(), "goleo-e2e-must-not-exist.bin")
	_ = os.Remove(deniedPath)

	// Written inside the app-data root, which fsAppDataDir grants, then read back.
	roundTripName := "e2e-binary.bin"

	var app *goleo.App
	app = goleo.New(goleo.Config{
		Title:        "bridge-e2e-verify",
		Width:        520,
		Height:       380,
		WindowMode:   goleo.WindowModeWebview,
		NativeIPC:    true,
		SchemeAssets: true,
		EmbedFS:      fe,
	})
	goleo.RegisterBuiltins(app.Bridge())
	goleo.RegisterFS(app.Bridge())

	app.Bridge().Handle("smoke:plan", func(ctx context.Context, args json.RawMessage) (any, error) {
		// Hand the payload over as plain numbers, NOT as []byte — Go marshals
		// []byte to base64, which is the very encoding under test here. Using it
		// to deliver the fixture would let a broken base64 agree with itself.
		nums := make([]int, len(binaryPayload))
		for i, b := range binaryPayload {
			nums[i] = int(b)
		}
		return map[string]any{
			"deniedPath":    deniedPath,
			"roundTripName": roundTripName,
			"payload":       nums,
		}, nil
	})

	done := make(chan bool, 1)

	app.Bridge().Handle("smoke:report", func(ctx context.Context, args json.RawMessage) (any, error) {
		fmt.Println("report:", string(args))
		var r report
		if err := json.Unmarshal(args, &r); err != nil {
			fmt.Println("RESULT: FAIL (bridge-e2e) — unreadable report:", err)
			done <- false
			return nil, nil
		}

		pass := true
		fail := func(format string, a ...any) {
			pass = false
			fmt.Printf("  FAIL: "+format+"\n", a...)
		}

		// 1. The bridge chose the native in-process channel, not the WebSocket
		//    fallback. If this is false the whole native path is dead code in
		//    production and every TS test has been exercising the wrong branch.
		if !r.Native {
			fail("the bridge did NOT use the native channel — it fell back to WebSocket")
		}

		// 2. Bytes survived the page->Go->page round-trip intact.
		if !r.BinaryOK {
			fail("binary round-trip corrupted the payload: got %s, want %x", r.BinaryHex, binaryPayload)
		}

		// 3. A denied write both failed AND left no file. "It threw" alone is not
		//    enough — it must not have written first.
		if !r.DeniedOK {
			fail("a write outside the FS roots did not report an error")
		}
		if _, err := os.Stat(deniedPath); err == nil {
			fail("the denied write actually created %s — confinement is not enforced", deniedPath)
			_ = os.Remove(deniedPath)
		}
		// The page must have seen the BACKEND's message, not fs.ts's
		// "requires the Go backend" mask.
		lowerErr := strings.ToLower(r.DeniedError)
		if r.DeniedError == "" {
			fail("no error text reached the page")
		} else if strings.Contains(lowerErr, "requires the go backend") {
			fail("fs.ts masked a real backend error as a missing-backend error: %q", r.DeniedError)
		} else if !strings.Contains(lowerErr, "outside") && !strings.Contains(lowerErr, "denied") {
			fail("error text does not look like the backend's confinement message: %q", r.DeniedError)
		}

		// 4. The grant path works: appDataDir() returns a writable root.
		if !r.AppDataOK {
			fail("could not write inside the app-data dir, which fsAppDataDir grants")
		}

		for _, f := range r.Failures {
			fail("page reported: %s", f)
		}

		if pass {
			fmt.Printf("RESULT: PASS (bridge-e2e) — real @goleo/bridge over native IPC from %s: "+
				"binary round-trip byte-exact, confinement enforced with the backend's own error, app-data grant works\n", r.Origin)
		} else {
			fmt.Println("RESULT: FAIL (bridge-e2e)")
		}
		done <- pass
		return nil, nil
	})

	go func() {
		ok := false
		select {
		case ok = <-done:
		case <-time.After(45 * time.Second):
			fmt.Println("RESULT: FAIL (bridge-e2e) — timeout, the page never reported")
		}
		time.Sleep(300 * time.Millisecond)
		app.Quit()
		if !ok {
			os.Exit(1)
		}
	}()

	app.Run()
}
