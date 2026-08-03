//go:build darwin && !ios

package notify

import (
	"strings"
	"testing"
)

// platformNotify builds AppleScript source from frontend-controlled strings
// (goleo:notify is a default builtin). Before escaping, a body containing
// `" & (do shell script "…") & "` closed the string literal and executed
// arbitrary commands. Assert the escaper neutralises the constructs that let a
// payload escape a quoted AppleScript literal.
func TestEscapeOSANeutralisesInjection(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"plain", "hello world"},
		{"double quote", `say "hi"`},
		{"the injection", `x" & (do shell script "touch /tmp/pwned") & "`},
		{"backslash", `C:\path\to`},
		{"escaped quote attempt", `x\" & beep & \"`},
		{"newline", "line1\nline2"},
		{"ampersand", "a & b"},
		{"unicode", "héllo → 世界"},
		{"empty", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := escapeOSA(c.in)

			// Must be a single quoted literal: one opening and one closing quote,
			// with no unescaped quote in between.
			if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
				t.Fatalf("escapeOSA(%q) = %q, want a quoted literal", c.in, got)
			}
			inner := got[1 : len(got)-1]
			for i := 0; i < len(inner); i++ {
				if inner[i] == '\\' {
					i++ // skip the escaped character
					continue
				}
				if inner[i] == '"' {
					t.Errorf("escapeOSA(%q) = %q leaves an unescaped quote at %d — the literal can be closed", c.in, got, i+1)
				}
			}
			// A raw newline would terminate the -e argument's statement.
			if strings.Contains(inner, "\n") {
				t.Errorf("escapeOSA(%q) = %q leaves a raw newline", c.in, got)
			}
		})
	}
}

// The escaper here is deliberately a copy of runtime/dialogs'. If they drift, the
// bug this fixed comes back in one of them, so pin the contract both rely on.
func TestEscapeOSAKnownVectors(t *testing.T) {
	if got, want := escapeOSA(`a"b`), `"a\"b"`; got != want {
		t.Errorf("escapeOSA(`a\"b`) = %s, want %s", got, want)
	}
	if got, want := escapeOSA(`a\b`), `"a\\b"`; got != want {
		t.Errorf("escapeOSA(`a\\b`) = %s, want %s", got, want)
	}
}
