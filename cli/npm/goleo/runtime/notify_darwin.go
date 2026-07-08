//go:build darwin && !ios

package runtime

import (
	"fmt"
	"os/exec"
	"strings"
)

func platformNotify(title, body string) error {
	script := fmt.Sprintf(`display notification "%s" with title "%s"`,
		appleScriptEscape(body), appleScriptEscape(title))

	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript notification failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func platformNotificationPermissionGranted() bool {
	// osascript notifications are delivered under the Script Editor
	// identity; there is no per-app permission to query here.
	return true
}

func platformRequestNotificationPermission() string {
	return "granted"
}

func appleScriptEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
