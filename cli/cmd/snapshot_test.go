package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// The mobile toolchain mutates go.mod/go.sum (adds golang.org/x/mobile);
// snapshotModFiles must restore the originals so a later -mod=vendor desktop
// build stays consistent.
func TestSnapshotModFiles(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	goSum := filepath.Join(dir, "go.sum")
	origMod := "module example.com/x\n\ngo 1.26\n\nrequire github.com/daforester/goleo v0.2.5\n"
	origSum := "github.com/daforester/goleo v0.2.5 h1:abc=\n"
	if err := os.WriteFile(goMod, []byte(origMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goSum, []byte(origSum), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := snapshotModFiles(dir)

	// Simulate the mobile toolchain polluting the module files.
	if err := os.WriteFile(goMod, []byte(origMod+"require golang.org/x/mobile v0.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goSum, []byte(origSum+"golang.org/x/mobile v0.0.0 h1:xyz=\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore()

	gotMod, _ := os.ReadFile(goMod)
	gotSum, _ := os.ReadFile(goSum)
	if string(gotMod) != origMod {
		t.Errorf("go.mod not restored:\n%s", gotMod)
	}
	if string(gotSum) != origSum {
		t.Errorf("go.sum not restored:\n%s", gotSum)
	}
}
