//go:build !darwin || mobilebuild || js

package runtime

import (
	"errors"
	"fmt"
)

// No native menu-bar backend outside macOS yet (Windows/Linux desktop apps
// typically use an in-page HTML menu; a native GTK/Win32 menu bar is future
// work). SetMenu guards on MenuSupported() before reaching this, so it is only
// hit if called directly.
func (a *App) setNativeMenu(items []MenuItem) error {
	return fmt.Errorf("goleo: native menu not implemented on this platform: %w", errors.ErrUnsupported)
}
