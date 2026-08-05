//go:build mobilebuild || js

package runtime

// No system tray on mobile / wasm. Background mode without a tray simply blocks
// on the quit context (see App.Run).
func (a *App) runTrayLoop() {}
