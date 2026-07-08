package runtime

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"
)

// AppUserModelID under which toasts are posted. Unpackaged apps need a
// registered AUMID; PowerShell's is always present, which is the standard
// trick for shell-based toasts (BurntToast, go-toast, tauri fallback).
const windowsToastAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

func platformNotify(title, body string) error {
	toastXML := fmt.Sprintf(
		`<toast activationType="protocol"><visual><binding template="ToastGeneric"><text>%s</text><text>%s</text></binding></visual></toast>`,
		xmlEscape(title), xmlEscape(body),
	)

	script := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$doc = New-Object Windows.Data.Xml.Dom.XmlDocument
$doc.LoadXml('%s')
$toast = New-Object Windows.UI.Notifications.ToastNotification($doc)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('%s').Show($toast)
`, psSingleQuoteEscape(toastXML), psSingleQuoteEscape(windowsToastAppID))

	cmd := exec.Command("powershell",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-EncodedCommand", encodePowerShellCommand(script),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("toast notification failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func platformNotificationPermissionGranted() bool {
	// Windows exposes no per-app query for unpackaged apps; toasts are
	// allowed unless the user disabled them system-wide.
	return true
}

func platformRequestNotificationPermission() string {
	return "granted"
}

func xmlEscape(s string) string {
	var buf strings.Builder
	xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

func psSingleQuoteEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// encodePowerShellCommand base64-encodes a script as UTF-16LE for
// powershell -EncodedCommand, avoiding all shell quoting issues.
func encodePowerShellCommand(script string) string {
	u16 := utf16.Encode([]rune(script))
	b := make([]byte, len(u16)*2)
	for i, v := range u16 {
		binary.LittleEndian.PutUint16(b[i*2:], v)
	}
	return base64.StdEncoding.EncodeToString(b)
}
