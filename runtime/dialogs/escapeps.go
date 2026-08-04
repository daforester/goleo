package dialogs

import "strings"

// escapePS makes a frontend-supplied string safe to interpolate into a
// SINGLE-quoted PowerShell string literal.
//
// Doubling `'` is both necessary and sufficient there: unlike double-quoted strings,
// a single-quoted PowerShell string performs no expansion, so `$`, “ ` “ and `@`
// are literal and cannot start a subexpression. Closing the quote is the only way
// out, and `”` is the escape for it.
//
// Kept free of build constraints so it can be tested from any host. It is the last
// remaining place a dialog interpolates untrusted text into a script (the message
// boxes and pickers moved to direct Win32 calls), and it had no tests.
//
// Newlines are preserved as real newlines. A single-quoted PowerShell string may span
// lines, so this is both valid and what the caller wants — a multi-line message should
// render as multiple lines. It previously substituted a backtick-n escape, which is
// only meaningful in a DOUBLE-quoted string; inside single quotes it is literal, so
// every newline showed up in the dialog as the two characters `n.
func escapePS(s string) string {
	// Normalise CRLF so a Windows-authored message does not double-space, then keep
	// the newline itself.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "'", "''")
}
