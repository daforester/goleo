package cmd

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestPrependPath(t *testing.T) {
	sep := string(os.PathListSeparator)

	t.Run("prepends to existing PATH", func(t *testing.T) {
		env := []string{"FOO=bar", "PATH=/usr/bin" + sep + "/bin"}
		got := prependPath(env, "/new/bin")
		want := "PATH=/new/bin" + sep + "/usr/bin" + sep + "/bin"
		if got[1] != want {
			t.Errorf("got %q, want %q", got[1], want)
		}
		if got[0] != "FOO=bar" {
			t.Errorf("unrelated var mutated: %q", got[0])
		}
	})

	t.Run("matches PATH case-insensitively (Windows Path)", func(t *testing.T) {
		env := []string{"Path=C:\\Windows"}
		got := prependPath(env, "C:\\go\\bin")
		if !strings.HasPrefix(got[0], "Path=C:\\go\\bin"+sep) {
			t.Errorf("did not prepend to case-insensitive Path: %q", got[0])
		}
		if len(got) != 1 {
			t.Errorf("should not add a second PATH entry, got %d entries: %v", len(got), got)
		}
	})

	t.Run("adds PATH when absent", func(t *testing.T) {
		env := []string{"FOO=bar"}
		got := prependPath(env, "/new/bin")
		if len(got) != 2 || got[1] != "PATH=/new/bin" {
			t.Errorf("expected appended PATH entry, got %v", got)
		}
	})

	t.Run("empty dir is a no-op", func(t *testing.T) {
		env := []string{"PATH=/usr/bin"}
		got := prependPath(env, "")
		if len(got) != 1 || got[0] != "PATH=/usr/bin" {
			t.Errorf("expected unchanged env, got %v", got)
		}
	})
}

func TestExeName(t *testing.T) {
	got := exeName("gomobile")
	want := "gomobile"
	if runtime.GOOS == "windows" {
		want = "gomobile.exe"
	}
	if got != want {
		t.Errorf("exeName(gomobile) = %q, want %q", got, want)
	}
}
