package dialogs

import (
	"fmt"
	"strings"
)

// zenity argument construction, deliberately kept free of build constraints and of
// exec.
//
// It lives apart from dialogs_linux.go so it can be tested on any host. The
// property below is a security property with no observable effect in a passing
// dialog, so it is exactly the kind of thing that silently regresses — and a test
// guarding it must not require a Linux box with zenity and a display to run.
//
// The property: every frontend-supplied value is passed as a single argv element of
// the form `--flag=value`, never as a separate `"--flag", value` pair. zenity uses
// GNU option parsing, so a value in its own argv slot that begins with `--` (a title
// of "--help", or worse "--file-selection") is parsed as another flag rather than as
// the value. No shell is involved, so this is flag confusion rather than RCE — but it
// lets a frontend change which dialog appears, which for a file picker means changing
// what the user is invited to hand over.

func zenityOpenArgs(opts FileDialogOptions) []string {
	args := []string{"--file-selection", "--title=" + opts.Title}
	if opts.DefaultPath != "" {
		args = append(args, "--filename="+opts.DefaultPath)
	}
	if opts.Multiple {
		args = append(args, "--multiple")
	}
	return append(args, zenityFileFilters(opts.Filters)...)
}

func zenitySaveArgs(opts FileDialogOptions) []string {
	args := []string{"--file-selection", "--save", "--title=" + opts.Title}
	if opts.DefaultPath != "" {
		args = append(args, "--filename="+opts.DefaultPath)
	}
	return append(args, zenityFileFilters(opts.Filters)...)
}

func zenityFolderArgs(opts FileDialogOptions) []string {
	args := []string{"--file-selection", "--directory", "--title=" + opts.Title}
	if opts.DefaultPath != "" {
		args = append(args, "--filename="+opts.DefaultPath)
	}
	return args
}

func zenityMessageArgs(opts MessageBoxOptions) []string {
	kind := "--info"
	switch opts.Icon {
	case "error":
		kind = "--error"
	case "warning":
		kind = "--warning"
	case "question":
		kind = "--question"
	}
	args := []string{kind, "--title=" + opts.Title, "--text=" + opts.Message}
	if len(opts.Buttons) >= 2 {
		if opts.Buttons[0] == "Yes" || opts.Buttons[0] == "yes" {
			args = append(args, "--ok-label="+opts.Buttons[0], "--cancel-label="+opts.Buttons[1])
		}
	}
	return args
}

func zenityPromptArgs(opts PromptOptions) []string {
	args := []string{"--entry", "--title=" + opts.Title, "--text=" + opts.Message}
	if opts.DefaultValue != "" {
		args = append(args, "--entry-text="+opts.DefaultValue)
	}
	return args
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
