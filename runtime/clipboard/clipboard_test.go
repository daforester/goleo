//go:build !(android || ios) || goleo_clipboard

package clipboard

import (
	"errors"
	"fmt"
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
			// Retry only the failure mode CONTENTION produces — an empty read where
			// something was written. The clipboard is a global single-owner resource, so
			// another process (a clipboard manager, an RDP session, Office) can take
			// ownership between the write and the read, and then our format is simply
			// gone. requireClipboard has already established the machine is quiet, but
			// contention is bursty rather than constant.
			//
			// A non-empty read that differs is CORRUPTION, not theft — the quoting and
			// trimming bugs this matrix exists for — so it fails immediately without
			// retrying. And exhausting the retries still fails: a write that never lands
			// (or a read that always trims, which is what "   " -> "" would be) fails
			// every attempt, so the retry cannot hide a deterministic defect.
			const attempts = 4
			var got string
			for i := 1; ; i++ {
				if err := WriteText(tc.text); err != nil {
					t.Fatalf("WriteText(%q): %v", tc.text, err)
				}
				var err error
				got, err = ReadText()
				if err != nil {
					t.Fatalf("ReadText after writing %q: %v", tc.text, err)
				}
				if got == tc.text {
					return
				}
				if got != "" || tc.text == "" {
					t.Fatalf("round trip corrupted the payload:\n write %q\n  read %q", tc.text, got)
				}
				if i == attempts {
					break
				}
				t.Logf("attempt %d read back empty (clipboard likely taken by another "+
					"process); retrying", i)
			}
			t.Errorf("round trip read back empty %d times:\n write %q\n"+
				"  Either the write never lands, or the read drops the value — for a "+
				"whitespace-only payload that is the trimming regression this case "+
				"exists to catch.", attempts, tc.text)
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
	restore := func() {
		if err := WriteText(original); err != nil {
			t.Logf("could not restore the original clipboard contents: %v", err)
		}
	}

	// The clipboard can be present and working and still be unusable for a strict
	// round-trip test, because it is a global single-owner resource: a clipboard
	// manager or remote-desktop session that grabs ownership on every change will
	// steal our value between the write and the read. Observed on a real Windows
	// machine, where roughly half of all round trips came back empty with a failing
	// set that SHIFTED between runs — contention, not a defect.
	//
	// So probe first, and note the asymmetry that keeps this guard honest:
	//
	//   all canaries pass  -> the machine is quiet; run the matrix strictly
	//   SOME canaries pass -> contended; skip, because the result would be noise
	//   NO canary passes   -> do NOT skip. A write that never lands is a real bug,
	//                         and contention essentially never blocks every attempt.
	//                         Fall through and let the matrix report it.
	//
	// That last branch is the point. A guard that skipped whenever the round trip
	// failed would hide exactly the regressions this file exists to catch.
	const canaries = 8
	ok := 0
	for i := 0; i < canaries; i++ {
		want := fmt.Sprintf("goleo clipboard canary %d", i)
		if err := WriteText(want); err != nil {
			break
		}
		if got, err := ReadText(); err == nil && got == want {
			ok++
		}
	}
	if ok > 0 && ok < canaries {
		restore()
		t.Skipf("the system clipboard is being modified by another process on this machine "+
			"(%d of %d canary round trips survived), so a strict round-trip test would "+
			"report contention as failure. Close any clipboard manager to run it.", ok, canaries)
	}

	return restore
}
