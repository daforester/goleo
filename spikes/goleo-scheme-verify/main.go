// End-to-end verification of the goleo Config.SchemeAssets integration — the
// REAL runtime path, not the raw-glaze/glaze-API spikes. One goleo app:
//
//   - Config.SchemeAssets: the primary window loads its embedded UI from the
//     portless custom origin goleo://app/ (served by the forked glaze scheme
//     handler wired through runtime/webview_glaze.go).
//   - Config.NativeIPC: the page talks to the Bridge over the in-process channel.
//
// Together that is a desktop window with NO TCP port in play, and the page must
// still report a SECURE CONTEXT (isSecureContext + localStorage + crypto.subtle)
// AND an origin of goleo://. Prints RESULT: PASS + exits 0 when confirmed. Run
// under xvfb on Linux (scripts/verify-linux-docker.*) and on macos-14
// (glaze-verify.yml).
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	goleo "github.com/daforester/goleo/runtime"
)

//go:embed all:frontend/dist
var fe embed.FS

func main() {
	var app *goleo.App
	app = goleo.New(goleo.Config{
		Title:        "scheme-verify",
		Width:        420,
		Height:       300,
		WindowMode:   goleo.WindowModeWebview,
		NativeIPC:    true,
		SchemeAssets: true, // <- the integration under test
		EmbedFS:      fe,
	})
	goleo.RegisterBuiltins(app.Bridge())

	app.Bridge().Handle("smoke:report", func(ctx context.Context, args json.RawMessage) (any, error) {
		fmt.Println("report:", string(args))
		var m struct {
			Origin              string
			Secure, LS, Crypto  bool
			Native              bool
			LSErr, CryptoErr    string
		}
		_ = json.Unmarshal(args, &m)

		// macOS/Linux serve the literal goleo:// scheme; Windows (WebView2) serves
		// it over the secure https://goleo.localhost virtual host (see the glaze
		// Windows backend). Accept either.
		schemeOrigin := strings.HasPrefix(m.Origin, "goleo://") || strings.HasPrefix(m.Origin, "https://goleo.localhost")
		pass := schemeOrigin && m.Secure && m.LS && m.Crypto && m.Native
		if pass {
			fmt.Printf("RESULT: PASS (SchemeAssets) — %q is a secure context over native IPC (no TCP port): ls+crypto ok\n", m.Origin)
		} else {
			fmt.Printf("RESULT: FAIL (SchemeAssets) — origin=%q schemeOrigin=%v secure=%v ls=%v crypto=%v native=%v\n",
				m.Origin, schemeOrigin, m.Secure, m.LS, m.Crypto, m.Native)
		}
		go func() {
			time.Sleep(300 * time.Millisecond)
			app.Quit()
			if !pass {
				os.Exit(1)
			}
		}()
		return nil, nil
	})

	// Safety net: never hang a CI runner.
	go func() {
		time.Sleep(30 * time.Second)
		fmt.Println("RESULT: FAIL (SchemeAssets) — timeout, no report")
		app.Quit()
		os.Exit(1)
	}()

	app.Run()
}
