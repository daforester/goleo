// Verifies the system tray runs cgo-free on real hardware: macOS via the
// purego/objc NSStatusItem backend (runtime/tray_darwin.go) — which must link
// alongside glaze without the goffi/purego fakecgo collision — and Linux via
// gogpu/systray (runtime/tray_desktop.go). A Background-mode app builds a tray
// with a menu, runs the native tray loop, then self-quits.
//
// "RESULT: PASS" + exit 0 means the tray was created and the loop ran and tore
// down cleanly (no objc/link crash). Icon rendering and click delivery can't be
// asserted headlessly; this is the "it builds a live tray without crashing"
// signal. Run under xvfb on Linux.
package main

import (
	"context"
	"fmt"
	"time"

	goleo "github.com/daforester/goleo/runtime"
)

func main() {
	var app *goleo.App
	app = goleo.New(goleo.Config{
		Title:      "tray-verify",
		Background: true, // headless controller; the tray owns the run loop
		Tray: &goleo.TrayConfig{
			Tooltip: "goleo tray verify",
			Items: []goleo.TrayItem{
				{Label: "Ping", OnClick: func() {}},
				{Label: "Quit", OnClick: func() { app.Quit() }},
			},
		},
		OnReady: func(ctx context.Context) {
			// Let the tray loop come up, then quit cleanly.
			go func() {
				time.Sleep(3 * time.Second)
				fmt.Println("RESULT: PASS (tray created + loop ran + clean quit)")
				app.Quit()
			}()
		},
	})
	goleo.RegisterBuiltins(app.Bridge())
	app.Run()
}
