//go:build darwin

package autostart

import (
	"os"
	"path/filepath"
)

func plistPath(appName string) (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	label := "com.goleo." + slug(appName)
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), label, nil
}

func platformEnable(appName, exePath string) error {
	path, label, err := plistPath(appName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(launchAgentPlist(label, exePath)), 0o644)
}

func platformDisable(appName string) error {
	path, _, err := plistPath(appName)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func platformIsEnabled(appName string) (bool, error) {
	path, _, err := plistPath(appName)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
