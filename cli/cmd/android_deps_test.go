package cmd

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeCmdlineToolsZip writes a zip mimicking Google's command-line tools
// archive: a top-level cmdline-tools/ directory with an executable bin/sdkmanager.
func makeCmdlineToolsZip(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	entries := []struct {
		name string
		mode os.FileMode
		body string
	}{
		{"cmdline-tools/bin/sdkmanager", 0755, "#!/bin/sh\nexit 0\n"},
		{"cmdline-tools/lib/sdkmanager.jar", 0644, "jar"},
	}
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name}
		hdr.SetMode(e.mode)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExtractCmdlineTools(t *testing.T) {
	installDir := t.TempDir()
	zipPath := filepath.Join(installDir, "cmdline-tools.zip")
	makeCmdlineToolsZip(t, zipPath)

	sdkmanager, err := extractCmdlineTools(zipPath, installDir)
	if err != nil {
		t.Fatalf("extractCmdlineTools: %v", err)
	}

	// Must be laid out as cmdline-tools/latest/bin/sdkmanager, not the doubled
	// cmdline-tools/cmdline-tools/bin/sdkmanager the old code left behind.
	want := filepath.Join(installDir, "cmdline-tools", "latest", "bin", "sdkmanager")
	if sdkmanager != want {
		t.Errorf("sdkmanager path = %q, want %q", sdkmanager, want)
	}

	info, err := os.Stat(sdkmanager)
	if err != nil {
		t.Fatalf("sdkmanager not present: %v", err)
	}

	// The doubled path must not exist.
	doubled := filepath.Join(installDir, "cmdline-tools", "cmdline-tools", "bin", "sdkmanager")
	if _, err := os.Stat(doubled); err == nil {
		t.Errorf("doubled cmdline-tools path unexpectedly exists: %s", doubled)
	}

	// The executable bit must survive extraction (POSIX only).
	if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		t.Errorf("sdkmanager is not executable: mode %v", info.Mode().Perm())
	}
}

func TestExtractCmdlineToolsClearsStaleDoubledLayout(t *testing.T) {
	installDir := t.TempDir()

	// Seed a stale, non-executable doubled layout like a pre-fix run left behind.
	staleBin := filepath.Join(installDir, "cmdline-tools", "cmdline-tools", "bin")
	if err := os.MkdirAll(staleBin, 0755); err != nil {
		t.Fatal(err)
	}
	staleSdkmanager := filepath.Join(staleBin, "sdkmanager")
	if err := os.WriteFile(staleSdkmanager, []byte("#!/bin/sh\nexit 0\n"), 0644); err != nil { // no +x
		t.Fatal(err)
	}

	zipPath := filepath.Join(installDir, "cmdline-tools.zip")
	makeCmdlineToolsZip(t, zipPath)
	sdkmanager, err := extractCmdlineTools(zipPath, installDir)
	if err != nil {
		t.Fatalf("extractCmdlineTools: %v", err)
	}

	// The stale doubled copy must be gone...
	if _, err := os.Stat(staleSdkmanager); err == nil {
		t.Errorf("stale doubled sdkmanager still present: %s", staleSdkmanager)
	}
	// ...and lookup must resolve the correct latest/ copy, executable.
	got := sdkmanagerPath(installDir)
	if got != sdkmanager {
		t.Errorf("sdkmanagerPath = %q, want %q", got, sdkmanager)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(got)
		if err != nil || info.Mode().Perm()&0111 == 0 {
			t.Errorf("resolved sdkmanager not executable: %v (err %v)", info.Mode().Perm(), err)
		}
	}
}

func TestSdkmanagerPathPrefersLatestOverDoubled(t *testing.T) {
	sdkRoot := t.TempDir()
	// Both a correct latest/ copy and a stale doubled copy exist.
	latestBin := filepath.Join(sdkRoot, "cmdline-tools", "latest", "bin")
	doubledBin := filepath.Join(sdkRoot, "cmdline-tools", "cmdline-tools", "bin")
	for _, d := range []string{latestBin, doubledBin} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "sdkmanager"), []byte("x"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	got := sdkmanagerPath(sdkRoot)
	want := filepath.Join(latestBin, "sdkmanager")
	if got != want {
		t.Errorf("sdkmanagerPath = %q, want the latest/ copy %q", got, want)
	}
}

func TestExtractCmdlineToolsIsIdempotent(t *testing.T) {
	installDir := t.TempDir()

	for i := 0; i < 2; i++ {
		zipPath := filepath.Join(installDir, "cmdline-tools.zip")
		makeCmdlineToolsZip(t, zipPath)
		if _, err := extractCmdlineTools(zipPath, installDir); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	// A stale extraction temp dir must not be left around.
	if _, err := os.Stat(filepath.Join(installDir, ".cmdline-tools-extract")); err == nil {
		t.Errorf("extraction temp dir was not cleaned up")
	}
}

func TestWindowsSdkmanagerCmdLine(t *testing.T) {
	got := windowsSdkToolCmdLine(
		`C:\Users\me\proj\.goleo\android\sdk\cmdline-tools\latest\bin\sdkmanager.bat`,
		[]string{"platforms;android-34", "ndk;25.2.9519653"},
	)
	want := `cmd /s /c "` +
		`"C:\Users\me\proj\.goleo\android\sdk\cmdline-tools\latest\bin\sdkmanager.bat" ` +
		`"platforms;android-34" "ndk;25.2.9519653""`
	if got != want {
		t.Errorf("cmd line mismatch:\n got: %s\nwant: %s", got, want)
	}
	// The package specs must remain quoted so cmd/batch don't split on ';'.
	if !strings.Contains(got, `"platforms;android-34"`) {
		t.Errorf("package spec not quoted intact: %s", got)
	}
}

func TestParseJavaMajor(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want int
		ok   bool
	}{
		{"java 26", `openjdk version "26.0.1" 2026-04-21`, 26, true},
		{"jdk 17", `openjdk version "17.0.9" 2023-10-17`, 17, true},
		{"jdk 21 no patch", `openjdk version "21" 2023-09-19`, 21, true},
		{"jdk 11", "openjdk version \"11.0.20\" 2023-07-18\nOpenJDK Runtime", 11, true},
		{"legacy 1.8", `java version "1.8.0_291"`, 8, true},
		{"legacy 1.7", `java version "1.7.0_80"`, 7, true},
		{"no quotes", "some unexpected output", 0, false},
		{"empty", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseJavaMajor(tt.out)
			if ok != tt.ok || (ok && got != tt.want) {
				t.Errorf("parseJavaMajor(%q) = (%d,%v), want (%d,%v)", tt.out, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestBuildJavaRange(t *testing.T) {
	// With the bundled Gradle 9.4.1 + AGP 9.2.0, JDK 17–26 are supported, so
	// these must be accepted...
	for _, v := range []int{17, 21, 23, 25, 26} {
		if v < minBuildJava || v > maxBuildJava {
			t.Errorf("Java %d should be accepted (range %d-%d)", v, minBuildJava, maxBuildJava)
		}
	}
	// ...and these (too old for AGP / too new for Gradle) must be rejected.
	for _, v := range []int{8, 11, 16, 27} {
		if v >= minBuildJava && v <= maxBuildJava {
			t.Errorf("Java %d should be rejected (range %d-%d)", v, minBuildJava, maxBuildJava)
		}
	}
}

func TestSystemImagePackage(t *testing.T) {
	got := systemImagePackage()
	wantArch := "x86_64"
	if runtime.GOARCH == "arm64" {
		wantArch = "arm64-v8a"
	}
	want := "system-images;android-34;google_apis;" + wantArch
	if got != want {
		t.Errorf("systemImagePackage() = %q, want %q", got, want)
	}
	// Must stay quotable/splittable-safe: the ';' separators are why runSdkTool
	// quotes args on Windows.
	if !strings.Contains(got, ";") {
		t.Errorf("expected package spec with ';' separators, got %q", got)
	}
}

func TestAvdmanagerPathFindsLatest(t *testing.T) {
	sdkRoot := t.TempDir()
	binDir := filepath.Join(sdkRoot, "cmdline-tools", "latest", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	name := "avdmanager"
	if runtime.GOOS == "windows" {
		name = "avdmanager.bat"
	}
	if err := os.WriteFile(filepath.Join(binDir, name), []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	got := avdmanagerPath(sdkRoot)
	want := filepath.Join(binDir, name)
	if got != want {
		t.Errorf("avdmanagerPath = %q, want %q", got, want)
	}
}

func TestWinQuote(t *testing.T) {
	if got := winQuote(`a b`); got != `"a b"` {
		t.Errorf("winQuote(a b) = %s", got)
	}
	// Embedded quotes are doubled for cmd.
	if got := winQuote(`a"b`); got != `"a""b"` {
		t.Errorf("winQuote(a\"b) = %s", got)
	}
}

func TestRunSdkmanagerUsesAbsolutePathRegardlessOfDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runSdkmanager uses sh; not applicable on Windows")
	}

	// A fake sdkmanager that records that it ran, at an ABSOLUTE path.
	toolDir := t.TempDir()
	sdkmanager := filepath.Join(toolDir, "sdkmanager")
	marker := filepath.Join(toolDir, "ran")
	script := "#!/bin/sh\ntouch " + marker + "\nexit 0\n"
	if err := os.WriteFile(sdkmanager, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	// SDKRoot is a *different* directory: this is exactly the situation that
	// broke with a relative sdkmanager path (cwd change -> doubled path -> 127).
	deps := &androidDeps{SDKRoot: t.TempDir()}
	if err := runSdkmanager(deps, sdkmanager, "ndk;25.2.9519653"); err != nil {
		t.Fatalf("runSdkmanager failed: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("sdkmanager did not run from SDKRoot cwd: %v", err)
	}
}
