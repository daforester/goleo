//go:build windows

package notify

import (
	"fmt"
	"os/exec"
	"strings"
)

func platformNotify(title, body string) error {
	script := fmt.Sprintf(`
Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime | Out-Null
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$textNodes = $template.GetElementsByTagName("text")
$textNodes.Item(0).AppendChild($template.CreateTextNode("%s")) | Out-Null
$textNodes.Item(1).AppendChild($template.CreateTextNode("%s")) | Out-Null
$toast = New-Object Windows.UI.Notifications.ToastNotification($doc)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('%s').Show($toast)
`, title, body, title)
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("toast notification failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func platformPermissionGranted() bool {
	return true
}

func platformRequestPermission() string {
	return "granted"
}
