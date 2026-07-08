//go:build windows

package dialogs

import (
	"fmt"
	"os/exec"
	"strings"
)

func platformOpenFile(opts FileDialogOptions) ([]string, error) {
	s, err := runPowerShellDialog(psOpenFileScript(opts))
	if err != nil {
		return nil, err
	}
	if s == "" {
		return nil, nil
	}
	paths := strings.Split(strings.TrimSpace(s), "\r\n")
	var out []string
	for _, p := range paths {
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

func platformSaveFile(opts FileDialogOptions) (string, error) {
	return runPowerShellDialog(psSaveFileScript(opts))
}

func platformSelectFolder(opts FileDialogOptions) (string, error) {
	return runPowerShellDialog(psSelectFolderScript(opts))
}

func platformShowMessage(opts MessageBoxOptions) (string, error) {
	icon := psMsgIcon(opts.Icon)
	btn := psMsgButtons(opts.Buttons)
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$r = [System.Windows.Forms.MessageBox]::Show('%s', '%s', %s, %s)
Write-Output $r
`, escapePS(opts.Message), escapePS(opts.Title), btn, icon)
	out, err := runPowerShell(script)
	if err != nil {
		return "OK", nil
	}
	return strings.TrimSpace(out), nil
}

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

func psOpenFileScript(opts FileDialogOptions) string {
	return fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = '%s'
$d.Multiselect = %s
$d.InitialDirectory = '%s'
%s
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
	$d.FileNames | ForEach-Object { Write-Output $_ }
}
`, escapePS(opts.Title), boolPS(opts.Multiple), escapePS(opts.DefaultPath), psFileFilters(opts.Filters))
}

func psSaveFileScript(opts FileDialogOptions) string {
	return fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.SaveFileDialog
$d.Title = '%s'
$d.InitialDirectory = '%s'
%s
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
	Write-Output $d.FileName
}
`, escapePS(opts.Title), escapePS(opts.DefaultPath), psFileFilters(opts.Filters))
}

func psSelectFolderScript(opts FileDialogOptions) string {
	return fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = '%s'
$d.SelectedPath = '%s'
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
	Write-Output $d.SelectedPath
}
`, escapePS(opts.Title), escapePS(opts.DefaultPath))
}

func psFileFilters(filters []FileFilter) string {
	if len(filters) == 0 {
		return `$d.Filter = "All Files (*.*)|*.*"`
	}
	var parts []string
	for _, f := range filters {
		pat := strings.Join(f.Patterns, ";")
		parts = append(parts, fmt.Sprintf("%s (%s)|%s", f.Name, pat, pat))
	}
	return fmt.Sprintf("$d.Filter = '%s'", escapePS(strings.Join(parts, "|")))
}

func psMsgIcon(icon string) string {
	switch icon {
	case "error":
		return "[System.Windows.Forms.MessageBoxIcon]::Error"
	case "warning":
		return "[System.Windows.Forms.MessageBoxIcon]::Warning"
	case "question":
		return "[System.Windows.Forms.MessageBoxIcon]::Question"
	default:
		return "[System.Windows.Forms.MessageBoxIcon]::Information"
	}
}

func psMsgButtons(btns []string) string {
	if len(btns) == 0 {
		return "[System.Windows.Forms.MessageBoxButtons]::OK"
	}
	switch len(btns) {
	case 1:
		return "[System.Windows.Forms.MessageBoxButtons]::OK"
	case 2:
		if btns[0] == "Yes" || btns[0] == "yes" {
			return "[System.Windows.Forms.MessageBoxButtons]::YesNo"
		}
		return "[System.Windows.Forms.MessageBoxButtons]::OKCancel"
	default:
		return "[System.Windows.Forms.MessageBoxButtons]::YesNoCancel"
	}
}

func runPowerShellDialog(script string) (string, error) {
	return runPowerShell(script)
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

func boolPS(b bool) string {
	if b {
		return "$true"
	}
	return "$false"
}

