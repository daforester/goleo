//go:build darwin && !mobilebuild && !js

package runtime

import "log"

// System tray is unavailable on macOS with the cgo-free glaze backend: both
// glaze (github.com/ebitengine/purego) and the tray (github.com/gogpu/systray →
// github.com/go-webgpu/goffi) ship a `fakecgo` that exports `_cgo_init`, and the
// Mach-O linker rejects the duplicate symbol (the ELF linker on Linux tolerates
// it, so the tray works there). Until the two share a single fakecgo (or the
// tray moves onto purego), macOS runs tray-less. See SPIKES.md.
//
// runTrayLoop therefore degrades to a headless controller: it keeps a
// Background app alive and tears down cleanly on Quit, matching the
// no-tray Background path in App.Run.
func (a *App) runTrayLoop() {
	if a.config.Tray != nil {
		log.Println("goleo: system tray is not available on macOS with the cgo-free backend; running headless")
	}
	<-a.ctx.Done()
	a.shutdown()
}
