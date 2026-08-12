package dialogs

import (
	"strings"
	"testing"
)

// platformShowMessage joined opts.Buttons into `buttons {%s}` raw — the one place
// in dialogs_darwin.go that skipped escapeOSA — so a frontend-supplied button
// label could close the AppleScript literal and inject. Every label must now be
// escaped, exactly like Message and Title.
func TestShowMessageButtonsAreEscaped(t *testing.T) {
	labels := []string{
		`OK`,
		`say "yes"`,
		`x" & (do shell script "touch /tmp/pwned") & "`,
		`back\slash`,
	}
	quoted := make([]string, 0, len(labels))
	for _, b := range labels {
		q := escapeOSA(b)
		if !strings.HasPrefix(q, `"`) || !strings.HasSuffix(q, `"`) {
			t.Fatalf("escapeOSA(%q) = %s, want a quoted literal", b, q)
		}
		quoted = append(quoted, q)
	}
	joined := strings.Join(quoted, ", ")

	// The assembled buttons list must not contain an unescaped quote, or the
	// enclosing `buttons {...}` clause can be terminated early.
	for i := 0; i < len(joined); i++ {
		if joined[i] == '\\' {
			i++
			continue
		}
		if joined[i] != '"' {
			continue
		}
		// A quote here is legal only as a literal delimiter: start of string, or
		// immediately after/before the ", " separator.
		atStart := i == 0
		atEnd := i == len(joined)-1
		beforeSep := strings.HasPrefix(joined[i:], `", `)
		afterSep := i >= 2 && joined[i-2:i] == ", "
		if !(atStart || atEnd || beforeSep || afterSep) {
			t.Errorf("buttons list %s has an unescaped quote at %d", joined, i)
		}
	}

	if !strings.Contains(joined, `\"`) {
		t.Error("expected at least one escaped quote from the injection payloads")
	}
	if strings.Contains(joined, `do shell script "touch`) {
		t.Error("the injection payload survived unescaped")
	}
}
