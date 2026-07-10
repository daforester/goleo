//go:build !mobilebuild && !js

package runtime

import (
	"os"

	"github.com/gogpu/systray"
)

// runTrayLoop builds the system tray from Config.Tray and blocks on its run
// loop (gogpu/systray owns the main thread). It is invoked from App.Run on the
// main goroutine in Background mode. Because the tray loop does not return on
// context cancel, a watcher goroutine performs the graceful teardown and exits
// the process when Quit is called.
func (a *App) runTrayLoop() {
	go func() {
		<-a.ctx.Done()
		a.shutdown()
		os.Exit(0)
	}()

	tray := systray.New()
	if a.config.Tray != nil {
		if len(a.config.Tray.Icon) > 0 {
			tray.SetIcon(a.config.Tray.Icon)
		}
		if a.config.Tray.Tooltip != "" {
			tray.SetTooltip(a.config.Tray.Tooltip)
		}
		if len(a.config.Tray.Items) > 0 {
			menu := systray.NewMenu()
			for _, item := range a.config.Tray.Items {
				it := item // capture
				menu.Add(it.Label, func() {
					if it.OnClick != nil {
						it.OnClick()
					}
				})
			}
			tray.SetMenu(menu)
		}
	}
	_ = tray.Run()
}
