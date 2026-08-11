//go:build windows

package notify

import (
	"syscall"
	"testing"
)

// A NUL in the notification text must not take the process down.
//
// copyUTF16 used syscall.StringToUTF16, which is deprecated for a reason that is easy to
// read past: it PANICS on a string containing a NUL rather than returning an error. Title
// and body reach here straight from goleo:notify, so they are app-supplied — a byte slice
// converted to a string, a fixed-width field read from a file, anything that carries a
// trailing NUL. The panic would come from inside a notification helper, which is about the
// last place anyone would look.
//
// This test runs only on Windows; the CI matrix cross-COMPILES for Windows but runs its
// tests on Linux, so it is a developer-machine guard rather than a gate.
func TestCopyUTF16SurvivesEmbeddedNUL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello", "hello"},
		{"trailing NUL", "hello\x00", "hello"},
		{"interior NUL", "he\x00llo", "hello"},
		{"only NULs", "\x00\x00", ""},
		{"non-ASCII survives", "héllo — ✓", "héllo — ✓"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dst := make([]uint16, 64)
			// Fails the test rather than crashing the run if the panic ever returns.
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("copyUTF16(%q) panicked: %v", c.in, r)
				}
			}()
			copyUTF16(dst, c.in)
			if got := syscall.UTF16ToString(dst); got != c.want {
				t.Errorf("copyUTF16(%q) wrote %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Truncation still has to leave a NUL terminator, or the shell reads past the field.
func TestCopyUTF16TruncatesTerminated(t *testing.T) {
	dst := make([]uint16, 5) // 4 chars + terminator
	copyUTF16(dst, "abcdefgh")
	if dst[len(dst)-1] != 0 {
		t.Errorf("truncated output is not NUL-terminated: %v", dst)
	}
	if got := syscall.UTF16ToString(dst); got != "abcd" {
		t.Errorf("truncated to %q, want %q", got, "abcd")
	}
}
