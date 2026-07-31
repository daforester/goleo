//go:build !(android || ios) || goleo_clipboard

package clipboard

import (
	"errors"
	"testing"
)

// TestRoundTrip covers the payloads that the old `powershell -Command
// Set-Clipboard <text>` write mangled or rejected outright — anything with a
// space failed with "A positional parameter cannot be found", text starting
// with "-" bound to a parameter name, and trailing whitespace was trimmed off
// on read.
func TestRoundTrip(t *testing.T) {
	restore := requireClipboard(t)
	defer restore()

	cases := []struct {
		name string
		text string
	}{
		{"plain", "nospace"},
		{"space", "hello world"},
		{"many spaces", "a b c d e"},
		{"leading and trailing space", "  padded  "},
		{"trailing newline", "line\n"},
		{"multiline", "first line\nsecond line\n\nfourth"},
		{"double quote", `say "hi there" now`},
		{"single quote", "it's a 'quoted' word"},
		{"backtick and dollar", "dollar $var and `backtick`"},
		{"semicolon and pipe", "one; two | three && four"},
		{"looks like a flag", "-Value not a parameter"},
		{"powershell subexpression", "$(Get-Date) stays literal"},
		{"unicode", "ünïcodé ✓ 日本語 🎉"},
		{"empty", ""},
		{"only whitespace", "   "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := WriteText(tc.text); err != nil {
				t.Fatalf("WriteText(%q): %v", tc.text, err)
			}
			got, err := ReadText()
			if err != nil {
				t.Fatalf("ReadText after writing %q: %v", tc.text, err)
			}
			if got != tc.text {
				t.Errorf("round trip mismatch:\n write %q\n  read %q", tc.text, got)
			}
		})
	}
}

// TestProviderTakesPrecedence checks that a registered native provider (what
// the mobile shell installs) short-circuits the platform implementation.
func TestProviderTakesPrecedence(t *testing.T) {
	p := &fakeProvider{}
	SetProvider(p)
	defer SetProvider(nil)

	if err := WriteText("via provider"); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	got, err := ReadText()
	if err != nil {
		t.Fatalf("ReadText: %v", err)
	}
	if got != "via provider" || p.text != "via provider" {
		t.Errorf("provider not used: stored %q, read %q", p.text, got)
	}
}

type fakeProvider struct{ text string }

func (f *fakeProvider) ReadText() (string, error) { return f.text, nil }
func (f *fakeProvider) WriteText(s string) error  { f.text = s; return nil }

// requireClipboard skips the test where there is no usable system clipboard
// (headless CI, no xclip, an unsupported GOOS) and otherwise returns a func
// that puts the user's clipboard back the way it was found.
func requireClipboard(t *testing.T) func() {
	t.Helper()
	SetProvider(nil)
	original, err := ReadText()
	if err != nil {
		if errors.Is(err, errors.ErrUnsupported) {
			t.Skipf("no clipboard on this platform: %v", err)
		}
		t.Skipf("system clipboard unavailable: %v", err)
	}
	return func() {
		if err := WriteText(original); err != nil {
			t.Logf("could not restore the original clipboard contents: %v", err)
		}
	}
}
