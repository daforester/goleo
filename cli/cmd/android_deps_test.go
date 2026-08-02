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

// writeFakeEmulator writes a fake `emulator` executable whose `-list-avds`
// prints the given names, one per line, and which otherwise exits 0.
func writeFakeEmulator(t *testing.T, dir string, avds ...string) string {
	t.Helper()
	name := "emulator"
	if runtime.GOOS == "windows" {
		name = "emulator.bat"
	}
	path := filepath.Join(dir, name)
	var script string
	if runtime.GOOS == "windows" {
		lines := "@echo off\n"
		for _, a := range avds {
			lines += "echo " + a + "\n"
		}
		script = lines
	} else {
		lines := "#!/bin/sh\n"
		for _, a := range avds {
			lines += "echo " + a + "\n"
		}
		lines += "exit 0\n"
		script = lines
	}
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeAVDConfig writes a fake <name>.avd/config.ini under avdHome, containing
// just the image.sysdir.1 key ensureAVD cares about.
func writeAVDConfig(t *testing.T, avdHome, name, sysdir string) {
	t.Helper()
	dir := filepath.Join(avdHome, name+".avd")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "avd.ini.encoding=UTF-8\nimage.sysdir.1=" + sysdir + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config.ini"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureAVDReusesWhenImagePresent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-only")
	}

	avdHome := t.TempDir()
	t.Setenv("ANDROID_AVD_HOME", avdHome)

	sysdir := "system-images/android-34/google_apis/x86_64"
	writeAVDConfig(t, avdHome, "goleo_avd", sysdir+"/")

	sdkRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sdkRoot, sysdir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdkRoot, sysdir, "system.img"), []byte("img"), 0644); err != nil {
		t.Fatal(err)
	}

	toolDir := t.TempDir()
	emuPath := writeFakeEmulator(t, toolDir, "goleo_avd")

	// A sdkmanager that would fail the test if invoked: the fast path must not
	// need it.
	sdkmanager := filepath.Join(toolDir, "sdkmanager")
	if err := os.WriteFile(sdkmanager, []byte("#!/bin/sh\ntouch "+filepath.Join(toolDir, "sdkmanager-ran")+"\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(sdkRoot, "cmdline-tools", "latest", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "sdkmanager"), []byte("#!/bin/sh\ntouch "+filepath.Join(toolDir, "sdkmanager-ran")+"\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}

	d := &androidDeps{SDKRoot: sdkRoot, EmulatorPath: emuPath}
	got, err := d.ensureAVD()
	if err != nil {
		t.Fatalf("ensureAVD: %v", err)
	}
	if got != "goleo_avd" {
		t.Errorf("ensureAVD() = %q, want %q", got, "goleo_avd")
	}
	if _, err := os.Stat(filepath.Join(toolDir, "sdkmanager-ran")); err == nil {
		t.Errorf("sdkmanager was invoked despite the system image already being present (unnecessary work)")
	}
}

func TestEnsureAVDReconcilesMissingImage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-only")
	}

	avdHome := t.TempDir()
	t.Setenv("ANDROID_AVD_HOME", avdHome)

	// This AVD was created (and its image installed) by a different project's
	// SDK; the current project's SDKRoot is a fresh tree with no matching
	// system-images directory at all.
	sysdir := "system-images/android-34/google_apis/x86_64"
	writeAVDConfig(t, avdHome, "goleo_avd", sysdir+"/")

	sdkRoot := t.TempDir() // fresh, no system-images present

	toolDir := t.TempDir()
	emuPath := writeFakeEmulator(t, toolDir, "goleo_avd")

	// Fake sdkmanager that records the package spec it was asked to install
	// and, to make the reconciliation observable end-to-end, actually lays
	// down a system.img at the path ensureAVD will re-check.
	sdkmanagerCalls := filepath.Join(toolDir, "sdkmanager-calls")
	binDir := filepath.Join(sdkRoot, "cmdline-tools", "latest", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	imgDir := filepath.Join(sdkRoot, sysdir)
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + sdkmanagerCalls + "\n" +
		"mkdir -p " + imgDir + "\n" +
		"touch " + filepath.Join(imgDir, "system.img") + "\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "sdkmanager"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	d := &androidDeps{SDKRoot: sdkRoot, EmulatorPath: emuPath}
	got, err := d.ensureAVD()
	if err != nil {
		t.Fatalf("ensureAVD: %v", err)
	}
	if got != "goleo_avd" {
		t.Errorf("ensureAVD() = %q, want %q", got, "goleo_avd")
	}

	callsRaw, err := os.ReadFile(sdkmanagerCalls)
	if err != nil {
		t.Fatalf("sdkmanager was not invoked to reconcile the missing image: %v", err)
	}
	calls := string(callsRaw)
	wantSpec := "system-images;android-34;google_apis;x86_64"
	if !strings.Contains(calls, wantSpec) {
		t.Errorf("sdkmanager invoked with %q, want it to include package spec %q", calls, wantSpec)
	}
}

func TestEnsureAVDReconciliationFailureIsAnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-only")
	}

	avdHome := t.TempDir()
	t.Setenv("ANDROID_AVD_HOME", avdHome)

	sysdir := "system-images/android-34/google_apis/x86_64"
	writeAVDConfig(t, avdHome, "goleo_avd", sysdir+"/")

	sdkRoot := t.TempDir()
	toolDir := t.TempDir()
	emuPath := writeFakeEmulator(t, toolDir, "goleo_avd")

	binDir := filepath.Join(sdkRoot, "cmdline-tools", "latest", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	// sdkmanager that always fails, e.g. no network to fetch the image.
	if err := os.WriteFile(filepath.Join(binDir, "sdkmanager"), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}

	d := &androidDeps{SDKRoot: sdkRoot, EmulatorPath: emuPath}
	got, err := d.ensureAVD()
	if err == nil {
		t.Fatalf("ensureAVD() = %q, nil error; want an error instead of silently proceeding with a broken AVD", got)
	}
}

func TestEnsureAVDTrustsUnparsableConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script tools are POSIX-only")
	}

	// AVD home has no config.ini at all for this AVD (foreign/corrupted AVD,
	// or one this test didn't bother faking a home directory for).
	avdHome := t.TempDir()
	t.Setenv("ANDROID_AVD_HOME", avdHome)

	toolDir := t.TempDir()
	emuPath := writeFakeEmulator(t, toolDir, "mystery_avd")

	d := &androidDeps{SDKRoot: t.TempDir(), EmulatorPath: emuPath}
	got, err := d.ensureAVD()
	if err != nil {
		t.Fatalf("ensureAVD: %v", err)
	}
	if got != "mystery_avd" {
		t.Errorf("ensureAVD() = %q, want %q (unparsable config.ini should be trusted, not blocked)", got, "mystery_avd")
	}
}

func TestSystemImagePackageFromSysdir(t *testing.T) {
	tests := []struct {
		sysdir string
		want   string
	}{
		{"system-images/android-34/google_apis/x86_64/", "system-images;android-34;google_apis;x86_64"},
		{"system-images/android-34/google_apis/x86_64", "system-images;android-34;google_apis;x86_64"},
		{"system-images\\android-34\\google_apis\\x86_64\\", "system-images;android-34;google_apis;x86_64"},
	}
	for _, tt := range tests {
		if got := systemImagePackageFromSysdir(tt.sysdir); got != tt.want {
			t.Errorf("systemImagePackageFromSysdir(%q) = %q, want %q", tt.sysdir, got, tt.want)
		}
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

func TestSharedAndroidCacheDirUsesUserCacheDir(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	want := filepath.Join(cacheHome, "goleo", "android")
	if got := sharedAndroidCacheDir(); got != want {
		t.Errorf("sharedAndroidCacheDir() = %q, want %q", got, want)
	}
}

func TestResolveSDKReusesSharedCacheOverCommonPaths(t *testing.T) {
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")

	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	sharedSDK := filepath.Join(cacheHome, "goleo", "android", "sdk")
	if err := os.MkdirAll(filepath.Join(sharedSDK, "cmdline-tools", "latest", "bin"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Chdir(t.TempDir())

	d := &androidDeps{}
	if err := d.resolveSDK(); err != nil {
		t.Fatalf("resolveSDK: %v", err)
	}
	want, err := filepath.Abs(sharedSDK)
	if err != nil {
		t.Fatal(err)
	}
	if d.SDKRoot != want {
		t.Errorf("resolveSDK() SDKRoot = %q, want shared cache dir %q", d.SDKRoot, want)
	}
}

func TestResolveSDKFallsBackToLegacyProjectLocalDir(t *testing.T) {
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()
	t.Chdir(projectDir)

	legacySDK := filepath.Join(projectDir, goleoAndroidDir, "sdk")
	if err := os.MkdirAll(filepath.Join(legacySDK, "cmdline-tools", "latest", "bin"), 0755); err != nil {
		t.Fatal(err)
	}

	d := &androidDeps{}
	if err := d.resolveSDK(); err != nil {
		t.Fatalf("resolveSDK: %v", err)
	}
	want, err := filepath.Abs(legacySDK)
	if err != nil {
		t.Fatal(err)
	}
	if d.SDKRoot != want {
		t.Errorf("resolveSDK() SDKRoot = %q, want legacy project-local dir %q", d.SDKRoot, want)
	}
}
