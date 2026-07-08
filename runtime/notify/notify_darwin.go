//go:build darwin && !ios

package notify

import (
	"fmt"
	"os/exec"
	"strings"
)

func platformNotify(title, body string) error {
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, body, title)
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript notification failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func platformPermissionGranted() bool {
	return true
}

func platformRequestPermission() string {
	return "granted"
}
