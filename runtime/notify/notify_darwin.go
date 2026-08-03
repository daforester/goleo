//go:build darwin && !ios

package notify

import (
	"fmt"
	"os/exec"
	"strings"
)

func platformNotify(title, body string) error {
	// title/body are frontend-controlled (goleo:notify is a default builtin), so
	// they MUST be escaped rather than interpolated into the AppleScript source.
	// Interpolating raw let a body containing `" & (do shell script "…") & "`
	// close the string literal and execute arbitrary commands.
	script := fmt.Sprintf(`display notification %s with title %s`, escapeOSA(body), escapeOSA(title))
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
