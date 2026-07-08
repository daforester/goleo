//go:build linux && !android

package dialogs

import (
	"fmt"
	"os/exec"
	"strings"
)

func platformOpenFile(opts FileDialogOptions) ([]string, error) {
	args := []string{"--file-selection", "--title", opts.Title}
	if opts.DefaultPath != "" {
		args = append(args, "--filename", opts.DefaultPath)
	}
	if opts.Multiple {
		args = append(args, "--multiple")
	}
	filters := zenityFileFilters(opts.Filters)
	args = append(args, filters...)

	out, err := runZenity(args...)
	if err != nil {
		return nil, nil
	}
	if opts.Multiple {
		return strings.Split(strings.TrimSpace(out), "|"), nil
	}
	return []string{strings.TrimSpace(out)}, nil
}

func platformSaveFile(opts FileDialogOptions) (string, error) {
	args := []string{"--file-selection", "--save", "--title", opts.Title}
	if opts.DefaultPath != "" {
		args = append(args, "--filename", opts.DefaultPath)
	}
	args = append(args, zenityFileFilters(opts.Filters)...)
	return runZenity(args...)
}

func platformSelectFolder(opts FileDialogOptions) (string, error) {
	args := []string{"--file-selection", "--directory", "--title", opts.Title}
	if opts.DefaultPath != "" {
		args = append(args, "--filename", opts.DefaultPath)
	}
	return runZenity(args...)
}

func platformShowMessage(opts MessageBoxOptions) (string, error) {
	var args []string
	switch opts.Icon {
	case "error":
		args = []string{"--error", "--title", opts.Title, "--text", opts.Message}
	case "warning":
		args = []string{"--warning", "--title", opts.Title, "--text", opts.Message}
	case "question":
		args = []string{"--question", "--title", opts.Title, "--text", opts.Message}
	default:
		args = []string{"--info", "--title", opts.Title, "--text", opts.Message}
	}
	if len(opts.Buttons) >= 2 {
		if opts.Buttons[0] == "Yes" || opts.Buttons[0] == "yes" {
			args = append(args, "--ok-label", opts.Buttons[0], "--cancel-label", opts.Buttons[1])
		}
	}
	_, err := runZenity(args...)
	if err != nil {
		return "Cancel", nil
	}
	if opts.Icon == "question" {
		return "Yes", nil
	}
	return "OK", nil
}

func platformShowPrompt(opts PromptOptions) (string, error) {
	args := []string{"--entry", "--title", opts.Title, "--text", opts.Message}
	if opts.DefaultValue != "" {
		args = append(args, "--entry-text", opts.DefaultValue)
	}
	out, err := runZenity(args...)
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

func zenityFileFilters(filters []FileFilter) []string {
	if len(filters) == 0 {
		return nil
	}
	var parts []string
	for _, f := range filters {
		pat := strings.Join(f.Patterns, " ")
		parts = append(parts, fmt.Sprintf("--file-filter=%s | %s", f.Name, pat))
	}
	return parts
}

func runZenity(args ...string) (string, error) {
	cmd := exec.Command("zenity", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("zenity error: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
