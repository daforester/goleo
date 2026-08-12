package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestBundleWindowsMakensis runs the real NSIS compiler end-to-end and asserts
// the installer lands at the expected absolute path — guarding against the
// dist\bundle\dist\bundle doubling regression (makensis cd's into the script
// dir, so a relative OutFile doubles). Skips where makensis is unavailable, so
// it is a no-op on non-Windows / CI without NSIS.
func TestBundleWindowsMakensis(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("NSIS bundle test runs on Windows")
	}
	if findMakensis() == "" {
		t.Skip("makensis not installed")
	}

	// installerName reads the package-global buildOutput; keep default naming.
	saved := buildOutput
	buildOutput = ""
	defer func() { buildOutput = saved }()

	dir := t.TempDir()
	outDir := filepath.Join(dir, "dist", "bundle")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A stand-in "built binary" for NSIS to package.
	binPath := filepath.Join(dir, "app.exe")
	if err := os.WriteFile(binPath, []byte("MZ stub binary payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := bundleConfig{
		AppName:     "Icon Demo",
		Version:     "1.2.3",
		Publisher:   "Acme Ltd",
		Description: "Goleo NSIS integration test",
		Copyright:   "© 2026 Acme Ltd",
	}
	if err := bundleWindows(binPath, cfg, outDir, signConfig{}); err != nil {
		t.Fatalf("bundleWindows: %v", err)
	}

	want := filepath.Join(outDir, "icon-demo-1.2.3-setup.exe")
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("installer not at expected path %s: %v", want, err)
	}
	if info.Size() < 1024 {
		t.Fatalf("installer suspiciously small (%d bytes)", info.Size())
	}
	// The doubled-path regression would put it here instead.
	if _, err := os.Stat(filepath.Join(outDir, "dist", "bundle")); err == nil {
		t.Fatalf("doubled output path regression: found dist\\bundle under outDir")
	}
	t.Logf("installer built: %s (%d bytes)", want, info.Size())
}
