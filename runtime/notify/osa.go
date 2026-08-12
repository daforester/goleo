package notify

import "strings"

// escapeOSA renders s as a quoted AppleScript string literal (quotes included).
//
// Deliberately NOT build-tagged to darwin even though only the darwin backend
// uses it: this is security-critical escaping — notification title/body are
// frontend-controlled through the default `goleo:notify` builtin — and gating it
// to darwin made its tests unrunnable anywhere except a Mac, which is how the
// unescaped interpolation it replaced survived in the first place. It is pure
// string manipulation, so it costs nothing on other platforms and is verified by
// the whole matrix on every one of them.
//
// Kept byte-identical to runtime/dialogs' helper; the two packages don't depend on
// each other. If you change one, change both.
func escapeOSA(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return `"` + s + `"`
}
