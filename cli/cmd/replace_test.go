package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSemverRe(t *testing.T) {
	// resolveVersion() returns e.g. "0.2.2" (no leading v) for a stamped release,
	// and "dev" otherwise.
	for _, v := range []string{"0.2.2", "1.0.0", "0.2.2-rc1"} {
		if !semverRe.MatchString(v) {
			t.Errorf("%q should be treated as a release version", v)
		}
	}
	for _, v := range []string{"dev", "v0.2.2", ""} {
		if semverRe.MatchString(v) {
			t.Errorf("%q should not be treated as a release version", v)
		}
	}
}

// Developing goleo itself (GOLEO_ROOT set) wires the local checkout in via a
// replace — no network, no proxy.
func TestEnsureLocalReplaceUsesGoleoRoot(t *testing.T) {
	proj := t.TempDir()
	goMod := "module example.com/x\n\ngo 1.26\n\nrequire github.com/daforester/goleo v0.2.2\n"
	if err := os.WriteFile(filepath.Join(proj, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	checkout := t.TempDir()
	if err := os.MkdirAll(filepath.Join(checkout, "runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "runtime", "app.go"), []byte("package runtime\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOLEO_ROOT", checkout)

	if err := ensureLocalReplace(proj); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(proj, "go.mod"))
	if !strings.Contains(string(data), "github.com/daforester/goleo =>") {
		t.Fatalf("expected a local replace to the GOLEO_ROOT checkout, got:\n%s", data)
	}
}
