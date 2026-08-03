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

// escapeOSA renders s as a quoted AppleScript string literal (quotes included).
// Deliberately duplicated from runtime/dialogs' identical helper: these feature
// packages don't depend on each other, and the one place that skipped escaping
// is what produced the injection above. Keep the two in sync.
func escapeOSA(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return `"` + s + `"`
}

func platformPermissionGranted() bool {
	return true
}

func platformRequestPermission() string {
	return "granted"
}
