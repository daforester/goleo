package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// newFrontendDirCmd builds a command carrying the same --frontend-dir flag the
// real dev/build commands declare, so Changed() behaves as it does in anger.
func newFrontendDirCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	var v string
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().StringVarP(&v, "frontend-dir", "f", "frontend", "")
	cmd.SetArgs(args)
	cmd.SetOut(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func TestResolveFrontendDirPrefersExplicitFlag(t *testing.T) {
	dir := t.TempDir()
	writeGoleoJSON(t, dir, `"frontend":{"directory":"web"}`)
	cmd := newFrontendDirCmd(t, "--frontend-dir", "explicit")
	if got := resolveFrontendDir(cmd, "explicit", dir); got != "explicit" {
		t.Errorf("resolveFrontendDir = %q, want the explicitly passed flag %q", got, "explicit")
	}
}

func TestResolveFrontendDirUsesConfigWhenFlagUnset(t *testing.T) {
	dir := t.TempDir()
	writeGoleoJSON(t, dir, `"frontend":{"directory":"."}`)
	cmd := newFrontendDirCmd(t)
	if got := resolveFrontendDir(cmd, "frontend", dir); got != "." {
		t.Errorf("resolveFrontendDir = %q, want goleo.json's %q", got, ".")
	}
}

func TestResolveFrontendDirFallsBackToFlagDefault(t *testing.T) {
	dir := t.TempDir() // no goleo.json at all
	cmd := newFrontendDirCmd(t)
	if got := resolveFrontendDir(cmd, "frontend", dir); got != "frontend" {
		t.Errorf("resolveFrontendDir = %q, want the flag default %q", got, "frontend")
	}
}

// emulate has no --frontend-dir flag, so it passes a nil command: config still
// applies, and the caller's default is the floor.
func TestResolveFrontendDirWithoutCommand(t *testing.T) {
	dir := t.TempDir()
	writeGoleoJSON(t, dir, `"frontend":{"directory":"ui"}`)
	if got := resolveFrontendDir(nil, "frontend", dir); got != "ui" {
		t.Errorf("resolveFrontendDir(nil) = %q, want %q", got, "ui")
	}
	if got := resolveFrontendDir(nil, "frontend", t.TempDir()); got != "frontend" {
		t.Errorf("resolveFrontendDir(nil) with no config = %q, want %q", got, "frontend")
	}
}

func TestAndroidBindTarget(t *testing.T) {
	original := buildAndroidABI
	t.Cleanup(func() { buildAndroidABI = original })

	for _, tc := range []struct {
		name string
		abi  string
		want string
	}{
		{"default builds every ABI", "", "android"},
		// gomobile's -target speaks GOARCH, not Android ABI names: passing
		// "x86_64" through unmapped fails with unsupported platform/arch.
		{"ABI name maps to GOARCH", "x86_64", "android/amd64"},
		{"arm64-v8a maps to arm64", "arm64-v8a", "android/arm64"},
		{"armeabi-v7a maps to arm", "armeabi-v7a", "android/arm"},
		{"x86 maps to 386", "x86", "android/386"},
		{"GOARCH accepted directly", "arm64", "android/arm64"},
		{"comma separated", "arm64-v8a,x86_64", "android/arm64,android/amd64"},
		{"tolerates spacing", " arm64-v8a , x86_64 ", "android/arm64,android/amd64"},
		{"deduplicates equivalent spellings", "x86_64,amd64", "android/amd64"},
		{"empty entries ignored", ",,", "android"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buildAndroidABI = tc.abi
			got, err := androidBindTarget()
			if err != nil {
				t.Fatalf("androidBindTarget() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("androidBindTarget() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAndroidBindTargetRejectsUnknownABI(t *testing.T) {
	original := buildAndroidABI
	t.Cleanup(func() { buildAndroidABI = original })

	for _, bad := range []string{"aarch64", "x64", "mips", "arm64-v8"} {
		buildAndroidABI = bad
		if _, err := androidBindTarget(); err == nil {
			t.Errorf("androidBindTarget() with %q = nil error, want a rejection", bad)
		}
	}
}

func TestValidGoArch(t *testing.T) {
	for _, ok := range []string{"amd64", "arm64", "386", "arm"} {
		if !validGoArch(ok) {
			t.Errorf("validGoArch(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "x86_64", "aarch64", "AMD64"} {
		if validGoArch(bad) {
			t.Errorf("validGoArch(%q) = true, want false", bad)
		}
	}
}
