//go:build windows

package dialogs

import (
	"fmt"
	"os/exec"
	"strings"
)

// The text prompt is the only dialog still implemented by shelling out to
// PowerShell.
//
// Message boxes and the file/folder pickers moved to direct Win32 calls — see
// dialogs_windows_native.go — because an unsigned binary spawning
// powershell.exe to run a script it just generated is the behaviour AV
// products flag hardest, and it got goleo's own CLI quarantined.
//
// There is no Win32 input-box primitive to move this one to: an input dialog
// has to be synthesised from an in-memory DLGTEMPLATE, which is materially
// more risk and code than the shell-out it would replace. The script is static
// apart from three escaped strings and runs -NoProfile -NonInteractive.

func platformShowPrompt(opts PromptOptions) (string, error) {
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$form = New-Object System.Windows.Forms.Form
$form.Text = '%s'
$form.Size = New-Object System.Drawing.Size(400,180)
$form.StartPosition = 'CenterScreen'
$label = New-Object System.Windows.Forms.Label
$label.Text = '%s'
$label.Location = New-Object System.Drawing.Point(10,20)
$label.Size = New-Object System.Drawing.Size(360,30)
$form.Controls.Add($label)
$textBox = New-Object System.Windows.Forms.TextBox
$textBox.Text = '%s'
$textBox.Location = New-Object System.Drawing.Point(10,60)
$textBox.Size = New-Object System.Drawing.Size(360,30)
$form.Controls.Add($textBox)
$okBtn = New-Object System.Windows.Forms.Button
$okBtn.Text = 'OK'
$okBtn.Location = New-Object System.Drawing.Point(100,110)
$okBtn.DialogResult = [System.Windows.Forms.DialogResult]::OK
$form.Controls.Add($okBtn)
$cancelBtn = New-Object System.Windows.Forms.Button
$cancelBtn.Text = 'Cancel'
$cancelBtn.Location = New-Object System.Drawing.Point(200,110)
$cancelBtn.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
$form.Controls.Add($cancelBtn)
$form.AcceptButton = $okBtn
$form.CancelButton = $cancelBtn
$result = $form.ShowDialog()
if ($result -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $textBox.Text }
`, escapePS(opts.Title), escapePS(opts.Message), escapePS(opts.DefaultValue))
	out, err := runPowerShell(script)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func runPowerShell(script string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("powershell error: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func escapePS(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "`n")
	return s
}
