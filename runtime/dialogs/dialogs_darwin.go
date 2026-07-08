//go:build darwin && !ios

package dialogs

import (
	"fmt"
	"os/exec"
	"strings"
)

func platformOpenFile(opts FileDialogOptions) ([]string, error) {
	args := []string{"-e", "choose file"}
	if opts.Multiple {
		args = append(args, "-e", "choose file with multiple selections allowed")
	}
	if opts.Title != "" {
		args = append(args, "with prompt", escapeOSA(opts.Title))
	}
	if opts.DefaultPath != "" {
		args = append(args, "default location", escapeOSA(opts.DefaultPath))
	}
	filters := osaFileFilters(opts.Filters)
	for _, f := range filters {
		args = append(args, "of type", f)
	}
	args = append(args, "-e", "POSIX path of result")

	out, err := osascript(args...)
	if err != nil {
		// user cancelled
		return nil, nil
	}
	lines := strings.Split(strings.TrimSpace(out), ",")
	var paths []string
	for _, p := range lines {
		if p != "" {
			paths = append(paths, strings.TrimSpace(p))
		}
	}
	return paths, nil
}

func platformSaveFile(opts FileDialogOptions) (string, error) {
	args := []string{"-e", "choose file name"}
	if opts.Title != "" {
		args = append(args, "with prompt", escapeOSA(opts.Title))
	}
	if opts.DefaultPath != "" {
		args = append(args, "default name", escapeOSA(opts.DefaultPath))
	}
	args = append(args, "-e", "POSIX path of result")
	return osascript(args...)
}

func platformSelectFolder(opts FileDialogOptions) (string, error) {
	args := []string{"-e", "choose folder"}
	if opts.Title != "" {
		args = append(args, "with prompt", escapeOSA(opts.Title))
	}
	if opts.DefaultPath != "" {
		args = append(args, "default location", escapeOSA(opts.DefaultPath))
	}
	args = append(args, "-e", "POSIX path of result")
	return osascript(args...)
}

func platformShowMessage(opts MessageBoxOptions) (string, error) {
	icon := osaMsgIcon(opts.Icon)
	btns := strings.Join(opts.Buttons, ", ")
	if btns == "" {
		btns = `"OK"`
	}
	script := fmt.Sprintf(`display dialog %s with title %s buttons {%s} default button 1 %s`,
		escapeOSA(opts.Message), escapeOSA(opts.Title), btns, icon)
	out, err := osascript("-e", script)
	if err != nil {
		// cancelled
		return "Cancel", nil
	}
	if strings.Contains(out, "button returned:") {
		parts := strings.SplitN(out, ":", 2)
		return strings.TrimSpace(parts[1]), nil
	}
	return "OK", nil
}

func platformShowPrompt(opts PromptOptions) (string, error) {
	script := fmt.Sprintf(`display dialog %s with title %s default answer %s`,
		escapeOSA(opts.Message), escapeOSA(opts.Title), escapeOSA(opts.DefaultValue))
	out, err := osascript("-e", script)
	if err != nil {
		return "", nil
	}
	if strings.Contains(out, "text returned:") {
		parts := strings.SplitN(out, ":", 2)
		return strings.TrimSpace(parts[1]), nil
	}
	return "", nil
}

func osaFileFilters(filters []FileFilter) []string {
	if len(filters) == 0 {
		return nil
	}
	var out []string
	for _, f := range filters {
		for _, p := range f.Patterns {
			p = strings.TrimPrefix(p, "*.")
			out = append(out, escapeOSA(`"`+p+`"`))
		}
	}
	return out
}

func osaMsgIcon(icon string) string {
	switch icon {
	case "error":
		return "with icon stop"
	case "warning":
		return "with icon caution"
	case "question":
		return "with icon note"
	default:
		return "with icon note"
	}
}

func osascript(args ...string) (string, error) {
	cmd := exec.Command("osascript", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("osascript error: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func escapeOSA(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return `"` + s + `"`
}
