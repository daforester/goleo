package dialogs

import (
	"strings"
	"testing"
)

// zenity uses GNU option parsing, so a frontend-supplied value in its own argv slot
// that starts with `--` is read as another flag instead of as the value. Passing
// everything as `--flag=value` binds it regardless of content.
//
// There is no shell here, so this is flag confusion rather than RCE — but for a file
// picker it lets a frontend change which dialog the user is shown, and therefore what
// they are invited to hand over. A passing dialog looks identical either way, which
// is exactly why it needs a test naming the property.
//
// These live in zenity_args_test.go rather than behind `//go:build linux` so they run
// on any host, not only on a Linux box with zenity and a display.

// Values a frontend could send that GNU parsing would treat as flags. Several
// deliberately collide with flags the builders legitimately emit — that collision is
// why the assertion below compares against a benign build instead of scanning for the
// literal, which was the first (wrong) formulation of this test.
var hostileValues = []string{
	"--help",
	"--file-selection",
	"--directory",
	"--multiple",
	"--save",
	"--filename=/etc/shadow",
	"--text=gotcha",
	"-h",
	"--",
}

const benign = "SAFE_VALUE"

// assertBound builds the argv twice — once with a harmless value, once with a hostile
// one — and requires that the only elements which differ are `--flag=value` pairs
// carrying it. Anything else means the value reached its own argv slot, or was split
// across slots, either of which lets zenity reinterpret it.
func assertBound(t *testing.T, what string, build func(string) []string, value string) {
	t.Helper()
	safe := build(benign)
	hostile := build(value)

	if len(safe) != len(hostile) {
		t.Errorf("%s: %q changed the argv shape (%d elements vs %d): %q",
			what, value, len(hostile), len(safe), hostile)
		return
	}

	carried := 0
	for i := range hostile {
		if hostile[i] == safe[i] {
			continue // a fixed flag the builder always emits
		}
		el := hostile[i]
		if !strings.HasPrefix(el, "--") || !strings.Contains(el, "=") {
			t.Errorf("%s: %q landed in a bare argv element %q — zenity would parse it as a flag (argv=%q)",
				what, value, el, hostile)
			continue
		}
		if !strings.Contains(el, value) {
			t.Errorf("%s: element %q neither matches the benign build nor carries %q (argv=%q)",
				what, el, value, hostile)
			continue
		}
		carried++
	}
	if carried == 0 {
		t.Errorf("%s: %q does not appear bound to any --flag=value element (argv=%q)",
			what, value, hostile)
	}
}

func TestZenityBindsHostileTitles(t *testing.T) {
	builders := map[string]func(string) []string{
		"open":    func(v string) []string { return zenityOpenArgs(FileDialogOptions{Title: v}) },
		"save":    func(v string) []string { return zenitySaveArgs(FileDialogOptions{Title: v}) },
		"folder":  func(v string) []string { return zenityFolderArgs(FileDialogOptions{Title: v}) },
		"message": func(v string) []string { return zenityMessageArgs(MessageBoxOptions{Title: v, Message: "m"}) },
		"prompt":  func(v string) []string { return zenityPromptArgs(PromptOptions{Title: v, Message: "m"}) },
	}
	for name, build := range builders {
		for _, v := range hostileValues {
			assertBound(t, name+"/title", build, v)
		}
	}
}

func TestZenityBindsHostileMessagesAndPaths(t *testing.T) {
	builders := map[string]func(string) []string{
		"message/text": func(v string) []string {
			return zenityMessageArgs(MessageBoxOptions{Title: "t", Message: v})
		},
		"prompt/text": func(v string) []string {
			return zenityPromptArgs(PromptOptions{Title: "t", Message: v})
		},
		"prompt/default": func(v string) []string {
			return zenityPromptArgs(PromptOptions{Title: "t", Message: "m", DefaultValue: v})
		},
		"open/defaultPath": func(v string) []string {
			return zenityOpenArgs(FileDialogOptions{Title: "t", DefaultPath: v})
		},
		"save/defaultPath": func(v string) []string {
			return zenitySaveArgs(FileDialogOptions{Title: "t", DefaultPath: v})
		},
		"folder/defaultPath": func(v string) []string {
			return zenityFolderArgs(FileDialogOptions{Title: "t", DefaultPath: v})
		},
	}
	for name, build := range builders {
		for _, v := range hostileValues {
			assertBound(t, name, build, v)
		}
	}
}

func TestZenityBindsHostileFilterNamesAndPatterns(t *testing.T) {
	byName := func(v string) []string {
		return zenityOpenArgs(FileDialogOptions{
			Title:   "t",
			Filters: []FileFilter{{Name: v, Patterns: []string{"*.txt"}}},
		})
	}
	byPattern := func(v string) []string {
		return zenityOpenArgs(FileDialogOptions{
			Title:   "t",
			Filters: []FileFilter{{Name: "Docs", Patterns: []string{v}}},
		})
	}
	for _, v := range hostileValues {
		assertBound(t, "filter/name", byName, v)
		assertBound(t, "filter/pattern", byPattern, v)
	}
}

// The dialogs must still be the ones asked for — a guard that mangled the request
// into a different dialog would satisfy the checks above.
func TestZenityArgsSelectTheRightDialog(t *testing.T) {
	for _, c := range []struct {
		name  string
		args  []string
		first string
	}{
		{"open", zenityOpenArgs(FileDialogOptions{Title: "t"}), "--file-selection"},
		{"save", zenitySaveArgs(FileDialogOptions{Title: "t"}), "--file-selection"},
		{"folder", zenityFolderArgs(FileDialogOptions{Title: "t"}), "--file-selection"},
		{"prompt", zenityPromptArgs(PromptOptions{Title: "t"}), "--entry"},
		{"info", zenityMessageArgs(MessageBoxOptions{Title: "t"}), "--info"},
		{"error", zenityMessageArgs(MessageBoxOptions{Title: "t", Icon: "error"}), "--error"},
		{"warning", zenityMessageArgs(MessageBoxOptions{Title: "t", Icon: "warning"}), "--warning"},
		{"question", zenityMessageArgs(MessageBoxOptions{Title: "t", Icon: "question"}), "--question"},
	} {
		if len(c.args) == 0 || c.args[0] != c.first {
			t.Errorf("%s: argv[0] = %q, want %q (argv=%q)", c.name, firstOr(c.args, ""), c.first, c.args)
		}
	}
	// save and folder are distinguished by a modifier, not by argv[0].
	if !contains(zenitySaveArgs(FileDialogOptions{}), "--save") {
		t.Error("save dialog is missing --save")
	}
	if !contains(zenityFolderArgs(FileDialogOptions{}), "--directory") {
		t.Error("folder dialog is missing --directory")
	}
	if !contains(zenityOpenArgs(FileDialogOptions{Multiple: true}), "--multiple") {
		t.Error("multi-select is missing --multiple")
	}
}

func firstOr(s []string, def string) string {
	if len(s) == 0 {
		return def
	}
	return s[0]
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
