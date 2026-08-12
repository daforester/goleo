//go:build linux && !android

package dialogs

import (
	"fmt"
	"os/exec"
	"strings"
)

// Argument construction lives in zenity_args.go, without a build constraint, so the
// `--flag=value` property it guards can be tested on any host.

func platformOpenFile(opts FileDialogOptions) ([]string, error) {
	out, err := runZenity(zenityOpenArgs(opts)...)
	if err != nil {
		return nil, nil
	}
	if opts.Multiple {
		return strings.Split(strings.TrimSpace(out), "|"), nil
	}
	return []string{strings.TrimSpace(out)}, nil
}

func platformSaveFile(opts FileDialogOptions) (string, error) {
	return runZenity(zenitySaveArgs(opts)...)
}

func platformSelectFolder(opts FileDialogOptions) (string, error) {
	return runZenity(zenityFolderArgs(opts)...)
}

func platformShowMessage(opts MessageBoxOptions) (string, error) {
	if _, err := runZenity(zenityMessageArgs(opts)...); err != nil {
		return "Cancel", nil
	}
	if opts.Icon == "question" {
		return "Yes", nil
	}
	return "OK", nil
}

func platformShowPrompt(opts PromptOptions) (string, error) {
	out, err := runZenity(zenityPromptArgs(opts)...)
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

func runZenity(args ...string) (string, error) {
	cmd := exec.Command("zenity", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("zenity error: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
