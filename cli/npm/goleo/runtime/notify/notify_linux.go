//go:build linux && !android

package notify

import (
	"fmt"
	"os/exec"
	"strings"
)

func platformNotify(title, body string) error {
	bin, err := exec.LookPath("notify-send")
	if err != nil {
		return fmt.Errorf("notify-send not found: install libnotify (e.g. apt install libnotify-bin)")
	}
	out, err := exec.Command(bin, title, body).CombinedOutput()
	if err != nil {
		return fmt.Errorf("notify-send failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func platformPermissionGranted() bool {
	_, err := exec.LookPath("notify-send")
	return err == nil
}

func platformRequestPermission() string {
	if platformPermissionGranted() {
		return "granted"
	}
	return "denied"
}
