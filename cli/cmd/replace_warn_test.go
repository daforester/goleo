package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A local-directory replace must be told apart from a module replace. The first is usually a
// leftover from GOLEO_ROOT and means the require version is decorative; the second is a
// deliberate fork pin and none of goleo's business.
//
// Go's own rule is the test: a directory replacement carries NO version, a module replacement
// must have one. That beats pattern-matching paths, which has to cope with ./, ../, /abs and
// Windows drive letters.
func TestLocalReplaceTargetOnlyMatchesDirectories(t *testing.T) {
	local := map[string]string{
		"windows absolute":  "replace " + goleoModule + " => E:/Development/goleo",
		"windows backslash": "replace " + goleoModule + ` => E:\Development\goleo`,
		"relative parent":   "replace " + goleoModule + " => ../goleo",
		"relative dot":      "replace " + goleoModule + " => ./vendor-src/goleo",
		"posix absolute":    "replace " + goleoModule + " => /home/me/goleo",
		"extra whitespace":  "  replace   " + goleoModule + "   =>   ../goleo  ",
		"trailing comment":  "replace " + goleoModule + " => ../goleo // my checkout",
	}
	for name, line := range local {
		got := localReplaceTarget(line, goleoModule)
		if got == "" {
			t.Errorf("%s: expected a directory target from %q, got none", name, line)
		}
		if strings.Contains(got, "//") {
			t.Errorf("%s: the trailing comment leaked into the target %q", name, got)
		}
	}

	// A module replacement is deliberate — never warn about it.
	notLocal := map[string]string{
		"fork pin":                 "replace " + goleoModule + " => github.com/someone/goleo v0.10.4",
		"fork pseudo":              "replace " + goleoModule + " => github.com/someone/goleo v0.0.0-20260101000000-abcdef123456",
		"commented out":            "// replace " + goleoModule + " => ../goleo",
		"different module":         "replace github.com/crgimenes/glaze => github.com/daforester/glaze v0.0.46-goleo.1",
		"a require, not a replace": "require " + goleoModule + " v0.10.4",
		"no replace at all":        "module example.com/app",
	}
	for name, line := range notLocal {
		if got := localReplaceTarget(line, goleoModule); got != "" {
			t.Errorf("%s: %q should not report a local directory, got %q", name, line, got)
		}
	}
}

// The state a real project was found in: a stale require plus a local replace, so the version
// in go.mod was decorative and the build was compiling a working tree several releases ahead.
func TestWarnsWhenGoleoIsReplacedByADirectoryAndGoleoRootIsUnset(t *testing.T) {
	dir := t.TempDir()
	goMod := `module example.com/app

go 1.26.5

replace github.com/crgimenes/glaze => github.com/daforester/glaze v0.0.46-goleo.1

require ` + goleoModule + ` v0.9.3

replace ` + goleoModule + ` => E:/Development/goleo
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOLEO_ROOT", "")
	out := captureStdout(t, func() { warnDetachedLocalReplace(dir) })

	for _, want := range []string{
		"E:/Development/goleo", // which directory
		"require",              // that the require is not what ships
		"GOLEO_ROOT",           // and that the variable is not set now
		"-dropreplace",         // how to fix it
		"go mod vendor",        // and that a vendored project needs re-vendoring
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the warning should mention %q:\n%s", want, out)
		}
	}
	// It must not warn about the glaze replace, which is a legitimate fork pin.
	if strings.Contains(out, "glaze") {
		t.Errorf("the warning flagged the glaze fork pin, which is deliberate:\n%s", out)
	}
}

// Silent when GOLEO_ROOT is set: the replace is intentional this run and the caller already
// prints "Using local goleo checkout".
func TestNoWarningWhileGoleoRootIsSet(t *testing.T) {
	dir := t.TempDir()
	goMod := "module example.com/app\n\nreplace " + goleoModule + " => ../goleo\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOLEO_ROOT", dir)
	if out := captureStdout(t, func() { warnDetachedLocalReplace(dir) }); out != "" {
		t.Errorf("expected silence while GOLEO_ROOT is set, got:\n%s", out)
	}
}

// Silent for an ordinary project — the overwhelmingly common case, and a warning here would be
// noise on every single build.
func TestNoWarningForAnOrdinaryProject(t *testing.T) {
	dir := t.TempDir()
	goMod := "module example.com/app\n\nrequire " + goleoModule + " v0.10.4\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOLEO_ROOT", "")
	if out := captureStdout(t, func() { warnDetachedLocalReplace(dir) }); out != "" {
		t.Errorf("expected silence for a project with no local replace, got:\n%s", out)
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, rerr := r.Read(buf)
			sb.Write(buf[:n])
			if rerr != nil {
				break
			}
		}
		done <- sb.String()
	}()
	fn()
	w.Close()
	os.Stdout = saved
	return <-done
}

// go.mod's block form. `go mod edit` never writes it, but a hand-edited file may, and the
// first version of localReplaceTarget only understood the single-line form.
func TestLocalReplaceTargetHandlesTheBlockForm(t *testing.T) {
	block := "module example.com/app\n\nreplace (\n\t" +
		goleoModule + " => ../goleo\n)\n"
	if got := localReplaceTarget(block, goleoModule); got != "../goleo" {
		t.Errorf("block form: got %q, want ../goleo", got)
	}

	// Same shape, but a module replacement — still deliberate, still no warning.
	blockModule := "module example.com/app\n\nreplace (\n\t" +
		goleoModule + " => github.com/someone/goleo v0.10.4\n)\n"
	if got := localReplaceTarget(blockModule, goleoModule); got != "" {
		t.Errorf("block form with a version should not report a directory, got %q", got)
	}

	// And a block that replaces a DIFFERENT module must not be attributed to goleo.
	blockOther := "module example.com/app\n\nreplace (\n\tgithub.com/crgimenes/glaze => ../glaze\n)\n"
	if got := localReplaceTarget(blockOther, goleoModule); got != "" {
		t.Errorf("another module's block replace was reported as goleo's: %q", got)
	}
}
