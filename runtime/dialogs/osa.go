package dialogs

import "strings"

// escapeOSA renders s as a quoted AppleScript string literal (quotes included).
//
// Deliberately NOT build-tagged to darwin even though only the darwin backend
// uses it: dialog titles, messages and button labels are frontend-controlled, so
// this is security-critical escaping, and gating it to darwin made its tests
// unrunnable anywhere but a Mac. That is precisely how platformShowMessage came to
// join opts.Buttons unescaped — the one call site in the darwin backend that
// skipped it — without any test catching it. Pure string manipulation, so it costs
// nothing elsewhere and is now verified on every platform.
//
// Kept byte-identical to runtime/notify's helper; the two packages don't depend on
// each other. If you change one, change both.
func escapeOSA(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return `"` + s + `"`
}
