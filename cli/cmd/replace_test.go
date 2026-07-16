package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// The npm launcher (bin/goleo.js) sets GOLEO_BUNDLE to the goleo module source
// bundled inside @goleo/cli; findGoleoRoot must honor it so scaffolded projects
// resolve github.com/daforester/goleo without it being published to the Go proxy.
func TestFindGoleoRootFromBundleEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "runtime", "app.go"), []byte("package runtime\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOLEO_ROOT", "") // don't let a dev override win
	t.Setenv("GOLEO_BUNDLE", dir)

	if got := findGoleoRoot(); got != dir {
		t.Fatalf("findGoleoRoot() = %q, want %q", got, dir)
	}
}

// A GOLEO_BUNDLE that doesn't actually contain the runtime is ignored (falls
// through), rather than being returned blindly.
func TestFindGoleoRootIgnoresBadBundle(t *testing.T) {
	t.Setenv("GOLEO_ROOT", "")
	t.Setenv("GOLEO_BUNDLE", t.TempDir()) // empty dir, no runtime/app.go
	if got := findGoleoRoot(); got == os.Getenv("GOLEO_BUNDLE") {
		t.Fatalf("findGoleoRoot() returned an invalid bundle dir %q", got)
	}
}
