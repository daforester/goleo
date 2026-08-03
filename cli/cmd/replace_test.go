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

func TestRequiredGoleoVersion(t *testing.T) {
	proj := t.TempDir()
	cases := []struct {
		name, content, want string
	}{
		{"standalone", "module x\n\ngo 1.26\n\nrequire github.com/daforester/goleo v0.8.3\n", "0.8.3"},
		{"block", "module x\n\ngo 1.26\n\nrequire (\n\tgithub.com/daforester/goleo v0.8.3\n\tother/module v1.0.0\n)\n", "0.8.3"},
		{"none", "module x\n\ngo 1.26\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(proj, "go.mod"), []byte(c.content), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := requiredGoleoVersion(proj); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestVendoredGoleoVersion(t *testing.T) {
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# github.com/daforester/goleo v0.8.3\n## explicit; go 1.26\n# other/module v1.0.0\n## explicit; go 1.26\n"
	if err := os.WriteFile(filepath.Join(proj, "vendor", "modules.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := vendoredGoleoVersion(proj); got != "0.8.3" {
		t.Errorf("got %q, want 0.8.3", got)
	}
}

// The core fix: an already-consistent vendored project must not attempt any
// network resolution on a later dev/build/emulate run, since a lagging module
// proxy/checksum DB (routine right after a fresh release) could otherwise
// re-pin go.mod to a version that disagrees with what's already vendored.
func TestEnsureLocalReplaceSkipsResolutionWhenVendorConsistent(t *testing.T) {
	proj := t.TempDir()
	goMod := "module example.com/x\n\ngo 1.26\n\nrequire github.com/daforester/goleo v0.8.3\n"
	if err := os.WriteFile(filepath.Join(proj, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	modulesTxt := "# github.com/daforester/goleo v0.8.3\n## explicit; go 1.26\n"
	if err := os.WriteFile(filepath.Join(proj, "vendor", "modules.txt"), []byte(modulesTxt), 0o644); err != nil {
		t.Fatal(err)
	}

	// No network available — if the fast path didn't trigger, ensureGoleoRequire's
	// `go get` would fail here and ensureLocalReplace would return an error.
	t.Setenv("GOPROXY", "off")
	if err := ensureLocalReplace(proj); err != nil {
		t.Fatalf("expected the already-consistent vendor to short-circuit network resolution, got: %v", err)
	}
}

// Sanity check on the other side: when go.mod and vendor actively disagree,
// the fast path must NOT mask that — resolution should still be attempted
// (and here, offline, fail), rather than silently no-op-ing on a real
// inconsistency.
func TestEnsureLocalReplaceReconcilesWhenVendorInconsistent(t *testing.T) {
	proj := t.TempDir()
	goMod := "module example.com/x\n\ngo 1.26\n\nrequire github.com/daforester/goleo v0.8.1\n"
	if err := os.WriteFile(filepath.Join(proj, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	modulesTxt := "# github.com/daforester/goleo v0.8.3\n## explicit; go 1.26\n"
	if err := os.WriteFile(filepath.Join(proj, "vendor", "modules.txt"), []byte(modulesTxt), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOPROXY", "off")
	if err := ensureLocalReplace(proj); err == nil {
		t.Fatal("expected resolution to be attempted (and fail offline) when go.mod and vendor disagree")
	}
}
