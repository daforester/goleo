//go:build linux && !android

package autostart

import (
	"os"
	"path/filepath"
)

func desktopPath(appName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "autostart", slug(appName)+".desktop"), nil
}

func platformEnable(appName, exePath string) error {
	path, err := desktopPath(appName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(desktopEntry(appName, exePath)), 0o644)
}

func platformDisable(appName string) error {
	path, err := desktopPath(appName)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func platformIsEnabled(appName string) (bool, error) {
	path, err := desktopPath(appName)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
