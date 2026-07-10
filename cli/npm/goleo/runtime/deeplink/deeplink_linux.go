//go:build linux && !android

package deeplink

import (
	"os"
	"os/exec"
	"path/filepath"
)

func platformRegister(scheme, appName, exePath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	appsDir := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return err
	}
	desktopFile := slug(appName) + "-" + scheme + ".desktop"
	path := filepath.Join(appsDir, desktopFile)
	if err := os.WriteFile(path, []byte(desktopEntry(scheme, appName, exePath)), 0o644); err != nil {
		return err
	}
	// Best-effort registration with the desktop environment.
	exec.Command("xdg-mime", "default", desktopFile, "x-scheme-handler/"+scheme).Run()
	exec.Command("update-desktop-database", appsDir).Run()
	return nil
}
