package cmd

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	// Java versions the Android build toolchain can run on. The bundled Gradle
	// (see gradle-wrapper.properties; currently 9.4.1) runs on JDK 17–26, and
	// AGP (currently 9.2.0) requires at least 17. Keep this range in sync with
	// the Gradle version pinned in the Android templates: raise maxBuildJava
	// only to a value the bundled Gradle documents as supported, otherwise its
	// embedded Kotlin DSL compiler throws while parsing the java version.
	minBuildJava = 17
	maxBuildJava = 26
)

type androidDeps struct {
	JavaHome     string
	SDKRoot      string
	NDKDir       string
	Gomobile     string
	AdbPath      string
	EmulatorPath string
	// hostAudio caches the -allow-host-audio probe (see supportsHostAudio);
	// hostAudioProbed distinguishes "probed and unsupported" from "not asked yet".
	hostAudio       bool
	hostAudioProbed bool
	// dryRun makes every resolve*() stop at "not found" instead of prompting
	// to install — used by `goleo doctor android` to report status without
	// side effects. Discovery (env vars, PATH, common paths, the project's
	// own .goleo/android/*) always runs first and is unaffected; only the
	// final interactive fallback is skipped.
	dryRun bool
}

const (
	goleoAndroidDir = ".goleo/android"
)

// sharedAndroidCacheDir returns a user-wide directory goleo can install the
// Android SDK/NDK/system-images and JDK into once, so every project on the
// machine finds and reuses the same install instead of each project
// downloading its own copy under its local .goleo/android. Falls back to
// the legacy project-local dir if the OS cache dir can't be determined
// (e.g. HOME unset) rather than failing outright.
func sharedAndroidCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		return goleoAndroidDir
	}
	return filepath.Join(base, "goleo", "android")
}

func ensureAndroidDeps() (*androidDeps, error) {
	deps := &androidDeps{}

	fmt.Println()
	fmt.Println("  Checking Android build dependencies...")

	if err := deps.resolveJava(); err != nil {
		return nil, err
	}
	if err := deps.resolveGomobile(); err != nil {
		return nil, err
	}
	if err := deps.resolveSDK(); err != nil {
		return nil, err
	}
	if err := deps.resolveNDK(); err != nil {
		return nil, err
	}
	if err := deps.resolveAdb(); err != nil {
		return nil, err
	}
	if err := deps.resolveEmulator(); err != nil {
		return nil, err
	}

	fmt.Println("  All Android dependencies resolved.")
	return deps, nil
}

// checkAndroidDeps runs every resolve*() in dry-run mode — discovery only,
// nothing installed, nothing prompted — and returns a status per dependency,
// in report order. Continues past a missing dependency instead of stopping at
// the first one like ensureAndroidDeps() does, since a status report wants
// the full picture, not just the first blocker. Backs `goleo doctor android`.
type androidDepStatus struct {
	name     string
	path     string // empty when err is non-nil
	err      error
	optional bool // missing doesn't fail the overall check (adb, emulator)
}

func checkAndroidDeps() (*androidDeps, []androidDepStatus) {
	d := &androidDeps{dryRun: true}
	statuses := []androidDepStatus{
		{name: "Java (JDK)"},
		{name: "gomobile"},
		{name: "Android SDK"},
		{name: "Android NDK"},
		{name: "adb", optional: true},
		{name: "Android emulator", optional: true},
	}
	statuses[0].err = d.resolveJava()
	statuses[0].path = d.JavaHome
	statuses[1].err = d.resolveGomobile()
	statuses[1].path = d.Gomobile
	statuses[2].err = d.resolveSDK()
	statuses[2].path = d.SDKRoot
	statuses[3].err = d.resolveNDK()
	statuses[3].path = d.NDKDir
	statuses[4].err = d.resolveAdb()
	statuses[4].path = d.AdbPath
	statuses[5].err = d.resolveEmulator()
	statuses[5].path = d.EmulatorPath
	return d, statuses
}

// avdStatus reports whether `emulate android` would find/reuse a working AVD
// without prompting, mirroring ensureAVD()'s reuse-fast-path check — read-only,
// installs nothing. Empty SDKRoot/EmulatorPath (already reported separately by
// checkAndroidDeps) means this can't be determined yet.
func (d *androidDeps) avdStatus() string {
	if d.EmulatorPath == "" {
		return "unknown (Android emulator not found — see above)"
	}
	existing := d.listAVDNames()
	if len(existing) == 0 {
		return "none found — first `emulate android` run will prompt to create one (~1 GB download)"
	}
	name := existing[0]
	sysdir := avdSystemImageSysdir(name)
	if sysdir == "" || d.SDKRoot == "" {
		return fmt.Sprintf("%q (unable to verify its system image; will be trusted as-is)", name)
	}
	if _, err := os.Stat(filepath.Join(d.SDKRoot, sysdir, "system.img")); err == nil {
		return fmt.Sprintf("%q (system image present under this project's SDK)", name)
	}
	return fmt.Sprintf("%q (system image missing from this project's SDK — will self-heal via sdkmanager on next emulate)", name)
}

func (d *androidDeps) resolveJava() error {
	// Collect candidate JAVA_HOMEs from the environment, PATH, and common
	// install locations, then pick the first one that has javac AND is a
	// Gradle-compatible version. A too-new system JDK (e.g. Java 26) is skipped
	// so we don't hand Gradle a JVM its Kotlin DSL compiler can't parse.
	var candidates []string
	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		candidates = append(candidates, jh)
	}
	// A JDK a previous goleo run downloaded, preferred so we don't re-download
	// when the only system JDK is incompatible. Check the shared, user-wide
	// cache first (what new installs use), then the legacy project-local dir
	// so projects set up before that change keep working without re-downloading.
	for _, root := range []string{filepath.Join(sharedAndroidCacheDir(), "jdk"), goleoAndroidDir + "/jdk"} {
		localRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if entries, err := os.ReadDir(localRoot); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					candidates = append(candidates, filepath.Join(localRoot, e.Name()))
				}
			}
		}
	}
	if path, err := exec.LookPath("javac"); err == nil {
		candidates = append(candidates, filepath.Dir(filepath.Dir(path)))
	}
	if path, err := exec.LookPath("java"); err == nil {
		parent := filepath.Dir(filepath.Dir(path))
		candidates = append(candidates, parent)
		for _, name := range []string{"jdk", "jdk-17", "jdk-21", "jdk-11"} {
			candidates = append(candidates, filepath.Join(parent, name))
		}
	}
	candidates = append(candidates, commonJavaPaths()...)

	seen := map[string]bool{}
	var incompatible []string
	for _, jh := range candidates {
		if jh == "" || seen[jh] {
			continue
		}
		seen[jh] = true
		if !hasJavac(jh) {
			continue
		}
		major, ok := javaMajorVersion(jh)
		if !ok {
			continue
		}
		if major >= minBuildJava && major <= maxBuildJava {
			d.JavaHome = jh
			return nil
		}
		incompatible = append(incompatible, fmt.Sprintf("%s (Java %d)", jh, major))
	}

	if len(incompatible) > 0 {
		fmt.Printf("  Installed JDK(s) are incompatible with the bundled Gradle (needs Java %d–%d): %s\n",
			minBuildJava, maxBuildJava, strings.Join(incompatible, ", "))
		fmt.Printf("  A compatible JDK %d will be used for the Android build.\n", minBuildJava)
	}

	return d.installJava()
}

// hasJavac reports whether javaHome contains a javac executable.
func hasJavac(javaHome string) bool {
	if javaHome == "" {
		return false
	}
	javac := filepath.Join(javaHome, "bin", "javac")
	if runtime.GOOS == "windows" {
		javac += ".exe"
	}
	_, err := os.Stat(javac)
	return err == nil
}

// javaMajorVersion runs `<javaHome>/bin/java -version` and returns the major
// Java version (e.g. 17, 21, 26; 8 for legacy "1.8" strings).
func javaMajorVersion(javaHome string) (int, bool) {
	javaBin := filepath.Join(javaHome, "bin", "java")
	if runtime.GOOS == "windows" {
		javaBin += ".exe"
	}
	out, err := exec.Command(javaBin, "-version").CombinedOutput()
	if err != nil {
		return 0, false
	}
	return parseJavaMajor(string(out))
}

// parseJavaMajor extracts the major version from `java -version` output, whose
// first quoted token looks like "26.0.1", "21", or the legacy "1.8.0_291".
func parseJavaMajor(verOutput string) (int, bool) {
	i := strings.IndexByte(verOutput, '"')
	if i < 0 {
		return 0, false
	}
	rest := verOutput[i+1:]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		return 0, false
	}
	fields := strings.FieldsFunc(rest[:j], func(r rune) bool {
		return r == '.' || r == '_' || r == '-' || r == '+'
	})
	if len(fields) == 0 {
		return 0, false
	}
	first, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, false
	}
	// Legacy "1.8" style: the major version is the second component.
	if first == 1 && len(fields) > 1 {
		if second, err := strconv.Atoi(fields[1]); err == nil {
			return second, true
		}
	}
	return first, true
}

func (d *androidDeps) installJava() error {
	if d.dryRun {
		return fmt.Errorf("not found — would prompt to auto-download JDK %d", minBuildJava)
	}
	fmt.Println()
	fmt.Println("  Java (JDK) not found.")
	fmt.Println("  Goleo can download and install it for you, or you can install it manually.")
	fmt.Println()
	fmt.Println("  Options:")
	fmt.Println("    1) Auto-download and install JDK 17")
	fmt.Println("    2) I'll install it myself (show instructions)")
	fmt.Println()

	var choice string
	fmt.Print("  Choose [1/2] (default 1): ")
	fmt.Scanln(&choice)

	if choice == "2" {
		fmt.Println()
		fmt.Println("  Install Java JDK manually:")
		fmt.Println("    Windows: https://adoptium.net/temurin/releases/?version=17")
		fmt.Println("    Linux:   sudo apt install openjdk-17-jdk")
		fmt.Println("    macOS:   brew install openjdk@17")
		fmt.Println()
		fmt.Println("  Then set JAVA_HOME to the installation directory.")
		return fmt.Errorf("Java JDK is required. Install it and re-run goleo emulate")
	}

	fmt.Println("  Downloading JDK 17...")
	installDir := filepath.Join(sharedAndroidCacheDir(), "jdk")
	os.MkdirAll(installDir, 0755)

	url := javaDownloadURL()
	archivePath := filepath.Join(installDir, "jdk.zip")

	if err := downloadFile(url, archivePath); err != nil {
		return fmt.Errorf("download failed: %w\nPlease install Java JDK manually and set JAVA_HOME", err)
	}

	fmt.Println("  Extracting JDK...")
	jdkDir, err := unzipAndFind(archivePath, installDir, func(name string) bool {
		return strings.Contains(name, "bin") && strings.Contains(name, "javac")
	})
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	d.JavaHome = jdkDir
	fmt.Printf("  Java JDK installed at: %s\n", jdkDir)

	os.Setenv("JAVA_HOME", jdkDir)
	return nil
}

func commonJavaPaths() []string {
	home, _ := os.UserHomeDir()
	paths := []string{}
	if runtime.GOOS == "windows" {
		paths = append(paths,
			"C:\\Program Files\\Java\\jdk-21",
			"C:\\Program Files\\Java\\jdk-17",
			"C:\\Program Files\\Java\\jdk-11",
			"C:\\Program Files\\Eclipse Adoptium\\jdk-21.0.1.12-hotspot",
			"C:\\Program Files\\Eclipse Adoptium\\jdk-17.0.9.9-hotspot",
			"C:\\Program Files\\Microsoft\\jdk-21.0.1-hotspot",
			home+"\\AppData\\Local\\Programs\\Eclipse Adoptium\\jdk-21.0.1.12-hotspot",
		)
	} else if runtime.GOOS == "darwin" {
		paths = append(paths,
			"/Library/Java/JavaVirtualMachines/jdk-21.jdk/Contents/Home",
			"/Library/Java/JavaVirtualMachines/jdk-17.jdk/Contents/Home",
			"/Library/Java/JavaVirtualMachines/jdk-11.jdk/Contents/Home",
			home+"/Library/Java/JavaVirtualMachines/jdk-21.jdk/Contents/Home",
			home+"/Library/Java/JavaVirtualMachines/jdk-17.jdk/Contents/Home",
		)
	} else {
		paths = append(paths,
			"/usr/lib/jvm/java-21-openjdk-amd64",
			"/usr/lib/jvm/java-17-openjdk-amd64",
			"/usr/lib/jvm/java-11-openjdk-amd64",
			"/usr/lib/jvm/java-21-openjdk",
			"/usr/lib/jvm/java-17-openjdk",
			"/usr/lib/jvm/java-11-openjdk",
			"/usr/lib/jvm/jdk-21",
			"/usr/lib/jvm/jdk-17",
		)
	}
	return paths
}

func javaDownloadURL() string {
	arch := runtime.GOARCH
	if arch == "x86_64" || arch == "amd64" {
		arch = "x64"
	} else if arch == "aarch64" || arch == "arm64" {
		arch = "aarch64"
	}
	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "mac"
	}
	return fmt.Sprintf("https://api.adoptium.net/v3/binary/version/jdk-17.0.9%%2B9/%s/%s/jdk/hotspot/normal/eclipse?project=jdk", osName, arch)
}

func (d *androidDeps) resolveGomobile() error {
	if path, ok := findTool("gomobile"); ok {
		d.Gomobile = path
		return nil
	}

	if d.dryRun {
		return fmt.Errorf("not found — would prompt to run: go install golang.org/x/mobile/cmd/gomobile@latest")
	}

	fmt.Println()
	fmt.Println("  gomobile not found.")
	fmt.Println("  Goleo can install it automatically with: go install golang.org/x/mobile/cmd/gomobile@latest")
	fmt.Println()
	fmt.Print("  Install gomobile now? [Y/n]: ")

	var choice string
	fmt.Scanln(&choice)
	if choice == "n" || choice == "N" {
		return fmt.Errorf("gomobile is required. Run: go install golang.org/x/mobile/cmd/gomobile@latest")
	}

	install := exec.Command("go", "install", "golang.org/x/mobile/cmd/gomobile@latest")
	install.Stdout = os.Stdout
	install.Stderr = os.Stderr
	if err := install.Run(); err != nil {
		return fmt.Errorf("gomobile install failed: %w\nRun manually: go install golang.org/x/mobile/cmd/gomobile@latest", err)
	}

	if path, ok := findTool("gomobile"); ok {
		d.Gomobile = path
		return nil
	}

	return fmt.Errorf("gomobile was installed but could not be located in the Go bin directory (%s) or on PATH", goBinDir())
}

func (d *androidDeps) resolveSDK() error {
	sdkRoot := os.Getenv("ANDROID_HOME")
	if sdkRoot == "" {
		sdkRoot = os.Getenv("ANDROID_SDK_ROOT")
	}

	if sdkRoot != "" {
		if _, err := os.Stat(filepath.Join(sdkRoot, "platforms")); err == nil {
			d.SDKRoot = absOr(sdkRoot)
			return nil
		}
	}

	// Reuse an SDK a previous goleo run installed, identified by the
	// command-line tools we lay down there. Check the shared, user-wide cache
	// first (what new installs use, and what lets every project on the
	// machine reuse a single download), then the legacy project-local .goleo
	// dir so projects set up before that change keep working without
	// re-downloading.
	for _, root := range []string{filepath.Join(sharedAndroidCacheDir(), "sdk"), goleoAndroidDir + "/sdk"} {
		local, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(local, "cmdline-tools", "latest", "bin")); err == nil {
			d.SDKRoot = local
			return nil
		}
	}

	home, _ := os.UserHomeDir()
	commonSDK := []string{
		filepath.Join(home, "Android", "Sdk"),
		filepath.Join(home, "android-sdk"),
		filepath.Join(home, ".android", "sdk"),
	}
	if runtime.GOOS == "windows" {
		commonSDK = append(commonSDK,
			filepath.Join(home, "AppData", "Local", "Android", "Sdk"),
			"C:\\Android\\Sdk",
			"C:\\Android\\android-sdk",
		)
	} else if runtime.GOOS == "darwin" {
		commonSDK = append(commonSDK,
			filepath.Join(home, "Library", "Android", "sdk"),
		)
	}

	for _, p := range commonSDK {
		if _, err := os.Stat(filepath.Join(p, "platforms")); err == nil {
			d.SDKRoot = absOr(p)
			return nil
		}
	}

	return d.installSDK()
}

// absOr returns the absolute form of p, or p unchanged if that fails. Keeping
// SDKRoot absolute matters because several sdkmanager calls run with a changed
// working directory.
func absOr(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

func (d *androidDeps) installSDK() error {
	if d.dryRun {
		return fmt.Errorf("not found — would prompt to download the command-line tools")
	}
	fmt.Println()
	fmt.Println("  Android SDK not found.")
	fmt.Println("  Goleo can download the command-line tools and set up the SDK automatically.")
	fmt.Println()
	fmt.Print("  Install Android SDK now? [Y/n]: ")

	var choice string
	fmt.Scanln(&choice)
	if choice == "n" || choice == "N" {
		return fmt.Errorf("Android SDK is required. Install it manually:\n  https://developer.android.com/studio#command-line-tools-only")
	}

	// Use an absolute install dir: several sdkmanager invocations run with a
	// changed working directory, so a relative path would be resolved against
	// the wrong cwd (producing a doubled, non-existent path). Installed to the
	// shared, user-wide cache (not the project-local .goleo dir) so every
	// project on the machine finds and reuses this same SDK/NDK/system-images
	// install instead of re-downloading its own copy.
	installDir, err := filepath.Abs(filepath.Join(sharedAndroidCacheDir(), "sdk"))
	if err != nil {
		return fmt.Errorf("resolving SDK install path: %w", err)
	}
	os.MkdirAll(installDir, 0755)

	fmt.Println("  Downloading Android command-line tools...")
	url := sdkManagerURL()
	archivePath := filepath.Join(installDir, "cmdline-tools.zip")

	if err := downloadFile(url, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	fmt.Println("  Extracting...")
	sdkmanager, err := extractCmdlineTools(archivePath, installDir)
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// SDKRoot must be set before running sdkmanager so the command picks up
	// ANDROID_HOME and the correct working directory.
	d.SDKRoot = installDir

	if os.Getenv("CI") != "" {
		fmt.Println("  Skipping interactive component install (CI).")
	} else {
		fmt.Println("  Installing essential SDK components via sdkmanager...")
		if err := runSdkmanager(d, sdkmanager, "platform-tools", "platforms;android-36", "build-tools;36.0.0"); err != nil {
			fmt.Printf("  Warning: sdkmanager component install failed: %v\n", err)
		}
	}

	fmt.Printf("  Android SDK installed at: %s\n", installDir)
	os.Setenv("ANDROID_HOME", installDir)
	return nil
}

// extractCmdlineTools unzips the Android command-line tools archive and lays it
// out as <installDir>/cmdline-tools/latest, the structure sdkmanager requires
// to compute the SDK root correctly (it treats two directories above its own
// bin/ as the SDK root). It returns the absolute path to the sdkmanager binary.
func extractCmdlineTools(archivePath, installDir string) (string, error) {
	tmp := filepath.Join(installDir, ".cmdline-tools-extract")
	os.RemoveAll(tmp)
	os.MkdirAll(tmp, 0755)
	defer os.RemoveAll(tmp)

	if _, err := unzipAndFind(archivePath, tmp, nil); err != nil {
		return "", err
	}

	// The archive contains a top-level "cmdline-tools" directory; fall back to
	// the extraction root if that ever changes.
	inner := filepath.Join(tmp, "cmdline-tools")
	if _, err := os.Stat(filepath.Join(inner, "bin")); err != nil {
		inner = tmp
	}

	// Remove the whole cmdline-tools directory first, not just latest/, so a
	// stale or doubled layout from an earlier run cannot linger and shadow the
	// correct binary during lookup.
	cmdlineTools := filepath.Join(installDir, "cmdline-tools")
	os.RemoveAll(cmdlineTools)
	latest := filepath.Join(cmdlineTools, "latest")
	if err := os.MkdirAll(cmdlineTools, 0755); err != nil {
		return "", err
	}
	if err := os.Rename(inner, latest); err != nil {
		// Rename can fail across filesystems; fall back to a per-entry move.
		os.MkdirAll(latest, 0755)
		if err := moveDir(inner, latest); err != nil {
			return "", err
		}
	}

	sdkmanager := filepath.Join(latest, "bin", sdkmanagerName())
	if _, err := os.Stat(sdkmanager); err != nil {
		return "", fmt.Errorf("sdkmanager not found after extraction at %s", sdkmanager)
	}
	// Guarantee the launcher is executable regardless of how the archive was
	// packed (no-op on Windows).
	if runtime.GOOS != "windows" {
		os.Chmod(sdkmanager, 0o755)
	}
	return sdkmanager, nil
}

func sdkmanagerName() string {
	if runtime.GOOS == "windows" {
		return "sdkmanager.bat"
	}
	return "sdkmanager"
}

// sdkmanagerPath returns the absolute path to sdkmanager within sdkRoot.
func sdkmanagerPath(sdkRoot string) string { return sdkToolPath(sdkRoot, "sdkmanager") }

// avdmanagerPath returns the absolute path to avdmanager within sdkRoot.
func avdmanagerPath(sdkRoot string) string { return sdkToolPath(sdkRoot, "avdmanager") }

// sdkToolPath returns the absolute path to a cmdline-tools launcher (base name
// without extension) within sdkRoot. It checks the well-known locations in
// priority order (rather than walking the tree, which could return a stale or
// doubled copy) and only accepts an existing regular file. Returns "" if none
// is found.
func sdkToolPath(sdkRoot, base string) string {
	name := base
	if runtime.GOOS == "windows" {
		name += ".bat"
	}
	candidates := []string{
		filepath.Join(sdkRoot, "cmdline-tools", "latest", "bin", name),
		filepath.Join(sdkRoot, "tools", "bin", name),
	}
	// Versioned command-line tools directories, e.g. cmdline-tools/11.0/bin.
	if entries, err := os.ReadDir(filepath.Join(sdkRoot, "cmdline-tools")); err == nil {
		for _, e := range entries {
			if e.IsDir() && e.Name() != "latest" {
				candidates = append(candidates, filepath.Join(sdkRoot, "cmdline-tools", e.Name(), "bin", name))
			}
		}
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return absOr(c)
		}
	}
	// Last resort: search the tree.
	if p := findFile(sdkRoot, base); p != "" {
		return absOr(p)
	}
	return ""
}

// runSdkmanager invokes sdkmanager for the given package specs, auto-accepting
// license prompts.
func runSdkmanager(d *androidDeps, sdkmanager string, pkgs ...string) error {
	// Accept package licenses without relying on `yes`/`sh`; feed enough "y"
	// responses for any realistic number of prompts.
	return runSdkTool(d, sdkmanager, strings.Repeat("y\n", 100), pkgs...)
}

// runSdkTool runs an Android command-line tool launcher (sdkmanager, avdmanager)
// with the given arguments, optionally feeding stdin to answer interactive
// prompts. The tool path must be absolute; the command runs from the SDK root
// and inherits JAVA_HOME/ANDROID_HOME so it works regardless of the caller's cwd
// or whether Java is on PATH. Command construction is platform-specific (see
// sdkToolCommand) so it works without a POSIX shell on Windows.
func runSdkTool(d *androidDeps, tool, stdin string, args ...string) error {
	cmd := sdkToolCommand(tool, args)
	cmd.Dir = d.SDKRoot
	setMobileEnv(cmd, d)
	if stdin != "" {
		// A spent in-memory reader is harmless if fewer responses are consumed.
		cmd.Stdin = strings.NewReader(stdin)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// windowsSdkToolCmdLine builds the raw cmd.exe command line used to run a
// cmdline-tools .bat launcher (sdkmanager, avdmanager) on Windows (.bat files
// cannot be executed directly via CreateProcess). Each argument is quoted so
// specs survive intact: cmd/batch treat ';' as an argument separator, which
// would otherwise split specs like "platforms;android-34". The whole command is
// wrapped in quotes and run with `/s` so cmd strips exactly the outer pair and
// runs the rest verbatim.
//
// Defined here (untagged) so it is unit-testable on any platform; the
// Windows-only wiring that consumes it lives in sdkmanager_windows.go.
func windowsSdkToolCmdLine(tool string, args []string) string {
	parts := []string{winQuote(tool)}
	for _, a := range args {
		parts = append(parts, winQuote(a))
	}
	return `cmd /s /c "` + strings.Join(parts, " ") + `"`
}

func winQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func (d *androidDeps) resolveNDK() error {
	// --android-ndk wins over everything. It was declared on both `goleo build` and
	// `goleo emulate`, documented in --help as "Path to Android NDK", and then never read
	// by anything: resolution went straight to ANDROID_NDK_HOME and autodiscovery. So
	// pointing it at a specific NDK silently used a different one, and when none was found
	// the error told you to "Set ANDROID_NDK_HOME manually" with the flag sitting unused
	// in the help output.
	//
	// A path given explicitly is an error if it is wrong, not something to fall back from:
	// silently using a different NDK than the one you named is how you get a build that
	// works on your machine and nowhere else.
	if flagNDK := strings.TrimSpace(buildAndroid); flagNDK != "" {
		if _, err := os.Stat(filepath.Join(flagNDK, "toolchains")); err != nil {
			return fmt.Errorf("--android-ndk %q does not look like an NDK "+
				"(no toolchains/ directory inside it): %w", flagNDK, err)
		}
		d.NDKDir = flagNDK
		return nil
	}

	ndkDir := os.Getenv("ANDROID_NDK_HOME")
	if ndkDir != "" {
		if _, err := os.Stat(filepath.Join(ndkDir, "toolchains")); err == nil {
			d.NDKDir = ndkDir
			return nil
		}
	}

	if d.SDKRoot != "" {
		ndkDir = filepath.Join(d.SDKRoot, "ndk")
		if entries, err := os.ReadDir(ndkDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					candidate := filepath.Join(ndkDir, e.Name())
					if _, err := os.Stat(filepath.Join(candidate, "toolchains")); err == nil {
						d.NDKDir = candidate
						return nil
					}
				}
			}
		}
		// Check within SDK/ndk-bundle
		legacyNdk := filepath.Join(d.SDKRoot, "ndk-bundle")
		if _, err := os.Stat(filepath.Join(legacyNdk, "toolchains")); err == nil {
			d.NDKDir = legacyNdk
			return nil
		}
	}

	home, _ := os.UserHomeDir()
	commonNDK := []string{
		filepath.Join(home, "Android", "Sdk", "ndk"),
		filepath.Join(home, "android-ndk"),
	}
	if runtime.GOOS == "windows" {
		commonNDK = append(commonNDK,
			filepath.Join(home, "AppData", "Local", "Android", "Sdk", "ndk"),
			"C:\\Android\\Sdk\\ndk",
		)
	}
	for _, p := range commonNDK {
		if entries, err := os.ReadDir(p); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					candidate := filepath.Join(p, e.Name())
					if _, err := os.Stat(filepath.Join(candidate, "toolchains")); err == nil {
						d.NDKDir = candidate
						return nil
					}
				}
			}
		}
	}

	if d.dryRun {
		return fmt.Errorf("not found — would prompt to install via sdkmanager")
	}

	// Offer to install via sdkmanager
	fmt.Println()
	fmt.Println("  Android NDK not found. It's needed for native code compilation.")
	fmt.Println()
	fmt.Print("  Install NDK via sdkmanager? [Y/n]: ")

	var choice string
	fmt.Scanln(&choice)
	if choice == "n" || choice == "N" {
		fmt.Println("  NDK must be installed manually. Install Android Studio or run:")
		fmt.Println("    sdkmanager --install \"ndk;25.2.9519653\"")
		return fmt.Errorf("Android NDK is required")
	}

	sdkmanager := sdkmanagerPath(d.SDKRoot)
	if sdkmanager == "" {
		fmt.Println("  sdkmanager not found. Cannot auto-install NDK.")
		fmt.Println("  Install Android Studio or download NDK manually:")
		fmt.Println("    https://developer.android.com/ndk/downloads")
		return fmt.Errorf("Android NDK is required")
	}

	fmt.Println("  Installing NDK via sdkmanager...")
	if err := runSdkmanager(d, sdkmanager, "ndk;25.2.9519653"); err != nil {
		return fmt.Errorf("NDK install failed: %w", err)
	}

	ndkDir = filepath.Join(d.SDKRoot, "ndk")
	if entries, err := os.ReadDir(ndkDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				d.NDKDir = filepath.Join(ndkDir, e.Name())
				return nil
			}
		}
	}

	fmt.Println("  NDK installed but path unknown. Set ANDROID_NDK_HOME manually.")
	return nil
}

func (d *androidDeps) resolveAdb() error {
	if path, err := exec.LookPath("adb"); err == nil {
		d.AdbPath = path
		return nil
	}

	home, _ := os.UserHomeDir()
	commonAdb := []string{
		filepath.Join(home, "Android", "Sdk", "platform-tools", "adb"),
	}
	if d.SDKRoot != "" {
		commonAdb = append(commonAdb,
			filepath.Join(d.SDKRoot, "platform-tools", "adb"),
		)
	}
	if runtime.GOOS == "windows" {
		adbName := "adb.exe"
		for i, p := range commonAdb {
			commonAdb[i] = p + ".exe"
		}
		commonAdb = append(commonAdb,
			filepath.Join(home, "AppData", "Local", "Android", "Sdk", "platform-tools", adbName),
			"C:\\Android\\Sdk\\platform-tools\\adb.exe",
		)
	}

	for _, p := range commonAdb {
		if _, err := os.Stat(p); err == nil {
			d.AdbPath = p
			return nil
		}
	}

	if d.dryRun {
		return fmt.Errorf("not found (optional: builds still work without it, but can't auto-deploy)")
	}

	fmt.Println()
	fmt.Println("  adb (Android Debug Bridge) not found in PATH.")
	fmt.Println("  It's included in Android SDK platform-tools.")
	if d.SDKRoot != "" {
		fmt.Printf("  Expected at: %s\n", filepath.Join(d.SDKRoot, "platform-tools", "adb"))
		fmt.Println()
		fmt.Print("  Install platform-tools now? [Y/n]: ")
		var choice string
		fmt.Scanln(&choice)
		if choice != "n" && choice != "N" {
			sdkmanager := sdkmanagerPath(d.SDKRoot)
			if sdkmanager != "" {
				if err := runSdkmanager(d, sdkmanager, "platform-tools"); err == nil {
					adbPath := filepath.Join(d.SDKRoot, "platform-tools", "adb")
					if runtime.GOOS == "windows" {
						adbPath += ".exe"
					}
					if _, err := os.Stat(adbPath); err == nil {
						d.AdbPath = adbPath
						return nil
					}
				}
			}
		}
	}

	fmt.Println("  adb not available. APK will be built but not deployed.")
	return nil
}

func (d *androidDeps) resolveEmulator() error {
	if path, err := exec.LookPath("emulator"); err == nil {
		d.EmulatorPath = path
		return nil
	}

	emulatorName := "emulator"
	if runtime.GOOS == "windows" {
		emulatorName = "emulator.exe"
	}

	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, "Android", "Sdk", "emulator", emulatorName),
	}
	if d.SDKRoot != "" {
		candidates = append(candidates,
			filepath.Join(d.SDKRoot, "emulator", emulatorName),
		)
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			filepath.Join(home, "AppData", "Local", "Android", "Sdk", "emulator", emulatorName),
			"C:\\Android\\Sdk\\emulator\\emulator.exe",
		)
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			d.EmulatorPath = p
			return nil
		}
	}

	if d.dryRun {
		return fmt.Errorf("not found (optional: only needed for `emulate android`)")
	}

	// Not installed. Offer to install the emulator package via sdkmanager.
	sdkmanager := sdkmanagerPath(d.SDKRoot)
	if d.SDKRoot == "" || sdkmanager == "" {
		return nil // nothing we can do; caller surfaces the missing-emulator error
	}

	fmt.Println()
	fmt.Println("  Android emulator not found.")
	fmt.Print("  Install the emulator via sdkmanager now? [Y/n]: ")
	var choice string
	fmt.Scanln(&choice)
	if choice == "n" || choice == "N" {
		return nil
	}

	fmt.Println("  Installing emulator via sdkmanager...")
	if err := runSdkmanager(d, sdkmanager, "emulator"); err != nil {
		fmt.Printf("  Warning: emulator install failed: %v\n", err)
		return nil
	}

	emuPath := filepath.Join(d.SDKRoot, "emulator", emulatorName)
	if _, err := os.Stat(emuPath); err == nil {
		d.EmulatorPath = emuPath
	}
	return nil
}

// avdName is the name of the AVD goleo creates when none exists.
const avdName = "goleo_avd"

// systemImagePackage returns the sdkmanager system-image spec matching the host
// architecture, used when creating an emulator AVD.
func systemImagePackage() string {
	arch := "x86_64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64-v8a"
	}
	return "system-images;android-34;google_apis;" + arch
}

// listAVDNames returns the names of installed AVDs, or nil if none / on error.
func (d *androidDeps) listAVDNames() []string {
	if d.EmulatorPath == "" {
		return nil
	}
	out, err := exec.Command(d.EmulatorPath, "-list-avds").Output()
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			names = append(names, s)
		}
	}
	return names
}

// avdHomeDir returns the directory AVDs are stored in, honoring
// ANDROID_AVD_HOME the same way the real Android tooling does. Falls back to
// the conventional ~/.android/avd when it's unset.
func avdHomeDir() string {
	if home := os.Getenv("ANDROID_AVD_HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".android", "avd")
}

// avdConfigPath returns where an AVD's config.ini lives. The file may not exist:
// `emulator -list-avds` enumerates the *.ini files at the AVD home root, which can
// name an AVD whose .avd directory is missing — a real state seen on a dev machine,
// where a stray emulator-5562.ini was listed while the only real AVD had no .ini at
// all. Callers that give advice about config.ini must check this separately from a
// key simply being absent, or they tell the user to edit a file that is not there.
func avdConfigPath(name string) string {
	return filepath.Join(avdHomeDir(), name+".avd", "config.ini")
}

// avdConfigValue reads <name>.avd/config.ini under the AVD home directory and
// returns the value recorded for key. Returns "" when the AVD directory,
// config.ini, or the key itself is absent — callers treat that as "unknown,
// can't validate" rather than "empty".
func avdConfigValue(name, key string) string {
	data, err := os.ReadFile(avdConfigPath(name))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(k) == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// avdSystemImageSysdir returns an AVD's image.sysdir.1 value (e.g.
// "system-images/android-34/google_apis/x86_64/", the path the emulator
// resolves relative to whichever SDK root it's launched with).
func avdSystemImageSysdir(name string) string {
	return avdConfigValue(name, "image.sysdir.1")
}

// supportsHostAudio reports whether this emulator accepts -allow-host-audio,
// the flag that lets the guest hear the host's microphone. Without it the
// emulator "zeroes out audio" (its own words), so a granted RECORD_AUDIO still
// yields no usable input device.
//
// Probed rather than assumed because an unrecognised flag makes the emulator
// refuse to start, which would break `goleo emulate android` outright for the
// sake of a microphone. `-help-<topic>` is unambiguous: exit 0 and a
// description for a known flag, exit 1 and "unknown option: ..." otherwise, so
// the exit status alone is enough and there is no output to parse.
//
// Cached: findDevice and `goleo doctor android` both ask.
func (d *androidDeps) supportsHostAudio() bool {
	if d.EmulatorPath == "" {
		return false
	}
	if !d.hostAudioProbed {
		d.hostAudioProbed = true
		d.hostAudio = exec.Command(d.EmulatorPath, "-help-allow-host-audio").Run() == nil
	}
	return d.hostAudio
}

// avdAudioStatus describes whether microphone capture can work on the emulator:
// the AVD needs hw.audioInput=yes (virtual mic hardware) AND the emulator has to
// pass the host's input through. Read-only, and never an error — a machine with
// no microphone is not a broken build environment.
func (d *androidDeps) avdAudioStatus() string {
	if d.EmulatorPath == "" {
		return "unknown (Android emulator not found — see above)"
	}
	hostAudio := d.supportsHostAudio()
	existing := d.listAVDNames()
	if len(existing) == 0 {
		if hostAudio {
			return "emulator can pass host audio through; no AVD yet to check for hw.audioInput"
		}
		return "this emulator does not support -allow-host-audio; microphone capture will be silent"
	}
	name := existing[0]
	// Distinguish "no config.ini" from "key absent". Telling someone to add
	// hw.audioInput to a file that does not exist sends them somewhere with nothing
	// in it — and a listed AVD with no .avd directory is a real state, not a
	// hypothetical (see avdConfigPath).
	if _, err := os.Stat(avdConfigPath(name)); err != nil {
		return fmt.Sprintf("%q has no readable config.ini at %s, so its audio settings "+
			"cannot be checked (the AVD may be registered but not actually present)",
			name, avdConfigPath(name))
	}
	switch avdConfigValue(name, "hw.audioInput") {
	case "yes":
		if hostAudio {
			return fmt.Sprintf("%q has hw.audioInput=yes and goleo will pass host audio through", name)
		}
		return fmt.Sprintf("%q has hw.audioInput=yes, but this emulator does not support "+
			"-allow-host-audio — capture will be silent", name)
	case "":
		return fmt.Sprintf("%q does not record hw.audioInput; microphone capture may not work "+
			"(add hw.audioInput=yes to its config.ini)", name)
	default:
		return fmt.Sprintf("%q has hw.audioInput disabled; microphone capture will not work "+
			"(set hw.audioInput=yes in its config.ini)", name)
	}
}

// systemImagePackageFromSysdir converts an image.sysdir.1 value as recorded
// in an AVD's config.ini (e.g. "system-images/android-34/google_apis/x86_64/")
// into the sdkmanager package spec that installs it
// ("system-images;android-34;google_apis;x86_64"). This is the standard
// Android SDK convention: a package spec's ';'-separated segments (after the
// first) are exactly its on-disk path components.
func systemImagePackageFromSysdir(sysdir string) string {
	// config.ini records this with forward slashes regardless of host OS, but
	// normalize backslashes too rather than assume that always holds.
	sysdir = strings.ReplaceAll(sysdir, "\\", "/")
	sysdir = strings.Trim(sysdir, "/")
	return strings.ReplaceAll(sysdir, "/", ";")
}

// ensureAVD returns the name of an AVD to boot, offering to create one (which
// downloads the matching system image) when none exist. Returns "" if none is
// available and the user declined or provisioning is not possible.
//
// goleo's SDK is private per-project (<project>/.goleo/android/sdk), but AVDs
// created via avdmanager land in the global, machine-wide ~/.android/avd (no
// --path/ANDROID_AVD_HOME override is used when creating them). Every goleo
// project on a machine therefore shares one AVD namespace while each has its
// own private SDK tree, so a reused AVD may point at a system image that was
// installed by, and only exists under, a different project's SDK root. Before
// trusting a reused AVD, verify its system image is actually present under
// this project's SDKRoot and self-heal (or fail loudly) if not.
func (d *androidDeps) ensureAVD() (string, error) {
	if existing := d.listAVDNames(); len(existing) > 0 {
		name := existing[0]
		sysdir := avdSystemImageSysdir(name)
		if sysdir == "" {
			// config.ini couldn't be found or parsed — a foreign/corrupted AVD
			// we can't introspect. Trust it as-is rather than blocking normal
			// usage on the common case (a perfectly healthy AVD whose home
			// directory we simply guessed wrong, or an old AVD format without
			// this key): the worst case is the pre-existing "reuse blindly"
			// behavior, not a new failure mode.
			return name, nil
		}
		if _, err := os.Stat(filepath.Join(d.SDKRoot, sysdir, "system.img")); err == nil {
			return name, nil // fast path: image already present under this project's SDK
		}

		// The AVD's system image is missing from this project's SDK root —
		// e.g. this AVD was created (and its image installed) by a different
		// goleo project sharing the global ~/.android/avd namespace. Self-heal
		// by installing the image THIS AVD needs, per its own config.ini (the
		// source of truth for an old/foreign AVD, which may not match today's
		// systemImagePackage()), into this project's SDKRoot.
		sdkmanager := sdkmanagerPath(d.SDKRoot)
		if sdkmanager == "" {
			return "", fmt.Errorf("AVD %q needs system image %q, which is not installed under %s, and sdkmanager could not be found to install it", name, sysdir, d.SDKRoot)
		}
		image := systemImagePackageFromSysdir(sysdir)
		fmt.Printf("  AVD %q needs system image %s, not found under this project's SDK; installing ...\n", name, image)
		if err := runSdkmanager(d, sdkmanager, image); err != nil {
			return "", fmt.Errorf("installing system image %s needed by AVD %q: %w", image, name, err)
		}
		return name, nil
	}

	sdkmanager := sdkmanagerPath(d.SDKRoot)
	avdmanager := avdmanagerPath(d.SDKRoot)
	if d.SDKRoot == "" || sdkmanager == "" || avdmanager == "" {
		return "", nil
	}

	fmt.Println()
	fmt.Println("  No Android Virtual Device (AVD) found.")
	fmt.Println("  Goleo can create one automatically. This downloads a system image (~1 GB).")
	fmt.Print("  Create a default AVD now? [Y/n]: ")
	var choice string
	fmt.Scanln(&choice)
	if choice == "n" || choice == "N" {
		return "", nil
	}

	image := systemImagePackage()
	fmt.Printf("  Installing system image %s ...\n", image)
	if err := runSdkmanager(d, sdkmanager, image); err != nil {
		return "", fmt.Errorf("system image install failed: %w", err)
	}

	fmt.Printf("  Creating AVD %q ...\n", avdName)
	// avdmanager prompts whether to create a custom hardware profile; answer no.
	if err := runSdkTool(d, avdmanager, strings.Repeat("no\n", 10),
		"create", "avd", "-n", avdName, "-k", image, "--device", "pixel", "--force"); err != nil {
		return "", fmt.Errorf("AVD creation failed: %w", err)
	}
	return avdName, nil
}

// Utility functions

func downloadFile(url, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, url)
	}

	_, err = io.Copy(out, resp.Body)
	return err
}

func unzipAndFind(zipPath, destDir string, predicate func(string) bool) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	var foundPath string
	for _, f := range reader.File {
		target := filepath.Join(destDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(target), 0755)

		src, err := f.Open()
		if err != nil {
			return "", err
		}

		// Preserve the archive's file mode so executables (sdkmanager, java,
		// javac, ...) keep their executable bit; os.Create would force 0666.
		mode := f.FileInfo().Mode().Perm()
		if mode == 0 {
			mode = 0644
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			src.Close()
			return "", err
		}

		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			return "", err
		}

		if predicate != nil && predicate(f.Name) && foundPath == "" {
			foundPath = filepath.Dir(target)
			// Walk up to find jdk root
			for i := 0; i < 3; i++ {
				parent := filepath.Dir(foundPath)
				if _, err := os.Stat(filepath.Join(parent, "bin", "javac")); err == nil {
					foundPath = parent
				} else {
					break
				}
			}
		}
	}

	os.Remove(zipPath)

	if predicate != nil {
		return foundPath, nil
	}
	return destDir, nil
}

func findFile(root, name string) string {
	var result string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Name() == name {
			result = path
			return io.EOF
		}
		if info.IsDir() && info.Name() == name {
			result = path
			return io.EOF
		}
		return nil
	})
	return result
}

func moveDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if err := os.Rename(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func sdkManagerURL() string {
	osName := runtime.GOOS
	var url string
	if osName == "windows" {
		url = "https://dl.google.com/android/repository/commandlinetools-win-11076708_latest.zip"
	} else if osName == "darwin" {
		url = "https://dl.google.com/android/repository/commandlinetools-mac-11076708_latest.zip"
	} else {
		url = "https://dl.google.com/android/repository/commandlinetools-linux-11076708_latest.zip"
	}
	return url
}
