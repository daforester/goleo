package cmd

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build [target]",
	Short: "Build the Goleo app for a target platform",
	Long: `Build the application for the specified target platform.

Targets:
  current     Build for the current platform (default)
  windows     Cross-compile for Windows (amd64)
  linux       Cross-compile for Linux (amd64)
  darwin      Cross-compile for macOS (amd64)
  android     Build Android .apk (requires gomobile and NDK)
  ios         Build iOS .xcframework (requires Xcode, macOS only)
  pwa         Build Progressive Web App (no Go backend)

The frontend is built first with Vite, then the Go backend
is compiled with the frontend assets embedded.

Examples:
  goleo build
  goleo build windows
  goleo build android`,
	Args: cobra.MaximumNArgs(1),
	RunE: runBuild,
}

var (
	buildOutput        string
	buildFrontend      string
	buildAndroid       string
	androidAPI         int
	iosDeployTarget    string
	buildBundle        bool
	buildPublish       bool
	buildArch          string
	buildAndroidABI    string
	buildNoSign        bool
	buildRelease       bool
	buildAndroidFormat string
	buildVersionCode   int
)

func init() {
	buildCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "Output file name")
	buildCmd.Flags().StringVarP(&buildFrontend, "frontend-dir", "f", "frontend", "Path to frontend directory (default: goleo.json frontend.directory)")
	buildCmd.Flags().StringVarP(&buildAndroid, "android-ndk", "", "", "Path to Android NDK")
	buildCmd.Flags().IntVarP(&androidAPI, "android-api", "", 24, "Android API level")
	buildCmd.Flags().StringVarP(&iosDeployTarget, "ios-target", "", "14.0", "iOS deployment target")
	buildCmd.Flags().BoolVar(&buildBundle, "bundle", false, "Package the built desktop app into a native installer (dist/bundle/)")
	buildCmd.Flags().BoolVar(&buildPublish, "publish", false, "Write an ed25519-signed update manifest for the built binary (needs GOLEO_UPDATE_PRIVKEY)")
	buildCmd.Flags().StringVar(&buildArch, "arch", "", "Target architecture for desktop targets, e.g. amd64 or arm64 (default: the target's own, or the host's for 'current')")
	buildCmd.Flags().StringVar(&buildAndroidABI, "android-abi", "", "Comma-separated Android ABIs to build: arm64-v8a, armeabi-v7a, x86_64, x86 (GOARCH names also accepted). Default: all four, ~4x the APK size")
	buildCmd.Flags().BoolVar(&buildNoSign, "no-sign", false, "Skip code signing even when signing credentials are configured")
	buildCmd.Flags().BoolVar(&buildRelease, "release", false, "Build a signed release artifact (Android: an .aab for Play; see --android-format)")
	buildCmd.Flags().StringVar(&buildAndroidFormat, "android-format", "", "Android artifact: aab or apk (default: aab with --release, apk otherwise)")
	buildCmd.Flags().IntVar(&buildVersionCode, "version-code", 0, "Override the Android versionCode (default: mobile.android.version_code, else derived from version)")
}

// androidArtifactFormat is the Android output kind.
type androidArtifactFormat string

const (
	androidFormatAPK androidArtifactFormat = "apk"
	androidFormatAAB androidArtifactFormat = "aab"
)

// resolveAndroidFormat picks the artifact format.
//
// Play requires an .aab for new uploads, so --release defaults to that; a debug build
// defaults to .apk because that is what you sideload with `adb install`. An explicit
// --android-format always wins, including `--release --android-format apk` for a signed
// APK to distribute outside a store.
func resolveAndroidFormat(flag string, release bool) (androidArtifactFormat, error) {
	switch strings.ToLower(strings.TrimSpace(flag)) {
	case "":
		if release {
			return androidFormatAAB, nil
		}
		return androidFormatAPK, nil
	case "aab":
		return androidFormatAAB, nil
	case "apk":
		return androidFormatAPK, nil
	default:
		return "", fmt.Errorf("unsupported --android-format %q (want aab or apk)", flag)
	}
}

// androidSigningConfigured reports whether the release signing environment is present.
//
// Only the keystore path is checked here; the passwords and alias are read inside
// build.gradle.kts (deliberately, so they never reach argv or gradle.properties) and
// Gradle reports a clear error itself if they are missing or wrong. Checking the path
// is enough to distinguish "the user meant to sign" from "the user has configured
// nothing".
func androidSigningConfigured() bool {
	return strings.TrimSpace(os.Getenv("GOLEO_ANDROID_KEYSTORE")) != ""
}

// androidGradleTask maps the build type and format to the Gradle task and the artifact
// Gradle will leave behind.
//
// The unsigned candidate matters: `assembleRelease` with no signingConfig emits
// app-release-unsigned.apk, not app-release.apk, so looking only for the latter would
// hit the "gradle succeeded but no artifact" path for a legitimate --no-sign build.
func androidGradleTask(format androidArtifactFormat, release bool) (task string, candidates []string) {
	switch {
	case format == androidFormatAAB && release:
		return "bundleRelease", []string{
			filepath.Join("app", "build", "outputs", "bundle", "release", "app-release.aab"),
		}
	case format == androidFormatAAB:
		return "bundleDebug", []string{
			filepath.Join("app", "build", "outputs", "bundle", "debug", "app-debug.aab"),
		}
	case release:
		return "assembleRelease", []string{
			filepath.Join("app", "build", "outputs", "apk", "release", "app-release.apk"),
			filepath.Join("app", "build", "outputs", "apk", "release", "app-release-unsigned.apk"),
		}
	default:
		return "assembleDebug", []string{
			filepath.Join("app", "build", "outputs", "apk", "debug", "app-debug.apk"),
		}
	}
}

type buildTarget struct {
	GOOS      string
	GOARCH    string
	OutputExt string
	Label     string
}

var targets = map[string]buildTarget{
	// "current" takes the HOST's GOOS, so its extension has to be derived rather
	// than fixed. It was hardcoded to "" — meaning `goleo build` on Windows, the
	// default command and the first thing a Windows user runs, produced a binary
	// named `app` with no `.exe`. Windows will not execute that: double-clicking
	// does nothing and Start-Process fails outright. Only the explicit
	// `goleo build windows` cross-target got it right.
	"current": {GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, OutputExt: hostOutputExt(), Label: "current"},
	"windows": {GOOS: "windows", GOARCH: "amd64", OutputExt: ".exe", Label: "Windows"},
	"linux":   {GOOS: "linux", GOARCH: "amd64", OutputExt: "", Label: "Linux"},
	"darwin":  {GOOS: "darwin", GOARCH: "amd64", OutputExt: "", Label: "macOS"},
	"android": {GOOS: "android", GOARCH: "arm64", OutputExt: ".aar", Label: "Android"},
	"ios":     {GOOS: "ios", GOARCH: "arm64", OutputExt: ".xcframework", Label: "iOS"},
	"pwa":     {GOOS: "js", GOARCH: "wasm", OutputExt: "", Label: "PWA"},
}

func runBuild(cmd *cobra.Command, args []string) error {
	targetName := "current"
	if len(args) > 0 {
		targetName = strings.ToLower(args[0])
	}

	target, ok := targets[targetName]
	if ok && buildArch != "" {
		// The named targets pin an arch (windows/linux/darwin => amd64) and
		// 'current' takes the host's, so cross-arch desktop builds previously
		// required a machine of that arch. gomobile targets carry their own
		// ABI handling — see --android-abi — so this stays desktop-only.
		if targetName == "android" || targetName == "ios" || targetName == "pwa" {
			return fmt.Errorf("--arch does not apply to the %s target (use --android-abi for Android ABIs)", targetName)
		}
		if !validGoArch(buildArch) {
			return fmt.Errorf("unsupported --arch %q (want one of: %s)", buildArch, strings.Join(knownGoArches, ", "))
		}
		target.GOARCH = buildArch
	}
	if !ok {
		return fmt.Errorf("unknown target: %s\nAvailable: current, windows, linux, darwin, android, ios, pwa", targetName)
	}

	buildFrontend = resolveFrontendDir(cmd, buildFrontend, ".")

	// Reject flags this target cannot honour, rather than accepting them and doing
	// nothing. The mobile and pwa paths return before the bundle/publish block, so
	// --bundle and --publish were silently ignored: you asked for an installer and a
	// signed update manifest, got neither, and the build reported success.
	//
	// They are refused rather than implemented because neither has a meaning here.
	// --bundle makes a native desktop installer, but on Android the APK/AAB IS the
	// installable artifact (--release --android-format aab is what you want), and on
	// PWA the output is a directory to host. --publish writes a manifest for goleo's
	// own self-updater, which mobile apps do not use — Play and the App Store handle
	// updates, and a sideloaded APK cannot replace its own running binary.
	if targetName == "android" || targetName == "ios" || targetName == "pwa" {
		for flag, set := range map[string]bool{"--bundle": buildBundle, "--publish": buildPublish} {
			if !set {
				continue
			}
			hint := "the built artifact is already the distributable one"
			if targetName == "android" {
				hint = "use --release (and --android-format aab|apk) for a store-ready artifact"
			}
			return fmt.Errorf("%s does not apply to the %s target — %s", flag, targetName, hint)
		}
	}
	// The Android-only flags should not look effective on a desktop build either.
	if targetName != "android" {
		if buildAndroidFormat != "" {
			return fmt.Errorf("--android-format only applies to the android target")
		}
		if buildVersionCode > 0 {
			return fmt.Errorf("--version-code only applies to the android target")
		}
		if buildRelease && targetName != "ios" {
			return fmt.Errorf("--release only applies to mobile targets; desktop builds use --bundle and the GOLEO_* signing variables")
		}
	}

	if targetName == "android" {
		// Validate BEFORE the frontend build and gomobile bind. Checking these where they
		// are used meant waiting through minutes of cross-compilation only to be told a
		// keystore was missing — or, for the package name, getting a javac error naming a
		// generated file rather than the goleo.json line responsible.
		if err := validateAndroidPackageName(loadMobileConfig(".").PackageName); err != nil {
			return err
		}
		if err := validateAndroidRelease(); err != nil {
			return err
		}
	}

	if err := checkGoleoJSON(); err != nil {
		return err
	}

	if targetName != "pwa" {
		if err := generateBackendEntrypoints("."); err != nil {
			return fmt.Errorf("generating backend entry points: %w", err)
		}
	}

	fmt.Printf("  Building Goleo app for %s (%s/%s)...\n", target.Label, target.GOOS, target.GOARCH)
	fmt.Println()

	distDirName := loadFrontendConfig(".").DistDir
	if distDirName == "" {
		distDirName = "dist"
	}
	frontendDist := filepath.Join(buildFrontend, distDirName)
	var extraEnv []string
	if targetName == "pwa" {
		extraEnv = append(extraEnv, "VITE_GOLEO_PLATFORM=pwa")
	}
	if err := buildFrontendProject(buildFrontend, frontendDist, extraEnv); err != nil {
		return fmt.Errorf("frontend build failed: %w", err)
	}

	if targetName == "android" {
		deps, err := ensureAndroidDeps()
		if err != nil {
			return err
		}
		return buildForAndroid(frontendDist, deps)
	}
	if targetName == "ios" {
		return buildForIOS(frontendDist)
	}
	if targetName == "pwa" {
		return buildForPWA(frontendDist)
	}

	if err := buildForDesktop(target, frontendDist); err != nil {
		return err
	}
	if buildBundle || buildPublish {
		binPath, _ := filepath.Abs(binaryOutputName(target))
		cfg := loadBundleConfig(".")
		if buildBundle {
			if err := runBundle(target, binPath, cfg); err != nil {
				return err
			}
		}
		if buildPublish {
			if err := runPublish(target, binPath, cfg); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildFrontendProject(frontendDir, distDir string, extraEnv []string) error {
	if _, err := os.Stat(filepath.Join(frontendDir, "package.json")); os.IsNotExist(err) {
		return fmt.Errorf("frontend directory not found: %s", frontendDir)
	}

	if _, err := os.Stat(filepath.Join(frontendDir, "node_modules")); os.IsNotExist(err) {
		fmt.Println("  Installing frontend dependencies...")
		install := exec.Command("npm", "install")
		install.Dir = frontendDir
		install.Stdout = os.Stdout
		install.Stderr = os.Stderr
		if err := install.Run(); err != nil {
			return fmt.Errorf("npm install failed: %w", err)
		}
	}

	build, usesCustomCommand := resolveBuildCommand(".", frontendDir, extraEnv)
	if usesCustomCommand {
		fmt.Printf("  Building frontend with %q...\n", strings.Join(build.Args, " "))
	} else {
		fmt.Println("  Building frontend with Vite...")
	}
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("frontend build failed: %w", err)
	}

	return nil
}

// resolveBuildCommand returns the command to build the frontend, and whether
// it's a project-supplied override. goleo.json lives at projectDir, which
// only equals frontendDir when the project is invoked with `-f .` — loaded
// from projectDir like every other goleo.json reader in this file
// (loadMobileConfig, loadBundleConfig), not frontendDir.
//
// When frontend.build_command is set, splits it on whitespace and execs it
// directly from frontendDir — no intermediary shell, same reasoning as
// resolveDevServer. Otherwise falls back to goleo's existing
// `npx vite build` from frontendDir, exactly as before.
func resolveBuildCommand(projectDir, frontendDir string, extraEnv []string) (*exec.Cmd, bool) {
	if buildCommand := loadFrontendConfig(projectDir).BuildCommand; buildCommand != "" {
		if parts := strings.Fields(buildCommand); len(parts) > 0 {
			cmd := exec.Command(parts[0], parts[1:]...)
			cmd.Dir = frontendDir
			cmd.Env = append(os.Environ(), extraEnv...)
			return cmd, true
		}
	}

	cmd := exec.Command("npx", "vite", "build")
	cmd.Dir = frontendDir
	cmd.Env = append(os.Environ(), extraEnv...)
	return cmd, false
}

// hostOutputExt is the executable extension for the host OS — the extension the
// "current" target must use, since it builds for whatever this machine is.
//
// Kept as a function rather than inlining `runtime.GOOS == "windows"` at the one
// call site so desktopOutputExt below can share it: every place that turns a GOOS
// into a file extension needs to agree, and they previously did not.
func hostOutputExt() string {
	return desktopOutputExt(runtime.GOOS)
}

// desktopOutputExt maps a GOOS to its executable extension.
func desktopOutputExt(goos string) string {
	if goos == "windows" {
		return ".exe"
	}
	return ""
}

// binaryOutputName is the built binary's file name: the -o value (default "app")
// plus the target's extension — without doubling it when -o already includes the
// extension (so `-o app.exe` yields app.exe, not app.exe.exe).
func binaryOutputName(target buildTarget) string {
	name := buildOutput
	if name == "" {
		name = "app"
	}
	if target.OutputExt != "" && !strings.EqualFold(filepath.Ext(name), target.OutputExt) {
		name += target.OutputExt
	}
	return name
}

func buildForDesktop(target buildTarget, distDir string) error {
	outputName := binaryOutputName(target)

	env := os.Environ()
	env = append(env, fmt.Sprintf("GOOS=%s", target.GOOS))
	env = append(env, fmt.Sprintf("GOARCH=%s", target.GOARCH))
	// All desktop targets are cgo-free via the purego glaze backend
	// (runtime/webview_glaze.go) — WKWebView (macOS), WebKitGTK (Linux) and
	// WebView2 (Windows) behind one binding — so every desktop build is
	// CGO_ENABLED=0 and cross-compiles from any host.
	env = append(env, "CGO_ENABLED=0")

	// The main package embeds frontend/dist relative to its own directory;
	// copy the built frontend there when the backend lives in backend/.
	pkgDir := backendPkgDir()
	if pkgDir == "./backend" && distExists(distDir) {
		embedDist := filepath.Join("backend", "frontend", "dist")
		os.RemoveAll(embedDist)
		os.MkdirAll(filepath.Dir(embedDist), 0755)
		if err := copyDir(distDir, embedDist); err != nil {
			return fmt.Errorf("copying frontend dist for embed: %w", err)
		}
	}

	cfg := loadBundleConfig(".")

	// Windows: embed the app icon + version info into the .exe (Details tab) from
	// goleo.json's bundle section. Best-effort — a failure leaves the default icon.
	if target.GOOS == "windows" {
		cleanup, err := writeWindowsResource(cfg, pkgDir, target.GOARCH)
		if err != nil {
			fmt.Println("  Warning: could not embed Windows icon/version info:", err)
		} else if cleanup != nil {
			defer cleanup()
			fmt.Println("  Embedding app icon + version info into the .exe")
		}
	}

	ldflags := fmt.Sprintf("-s -w -X main.Version=%s", cfg.Version)

	args := []string{"build", "-ldflags", ldflags}
	args = append(args, "-o", outputName, pkgDir)

	build := exec.Command("go", args...)
	build.Env = env
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr

	// Ensure all Go dependencies are resolved
	fmt.Println("  Resolving Go dependencies...")
	if err := ensureLocalReplace("."); err != nil {
		return fmt.Errorf("go module resolution: %w", err)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Stdout = os.Stdout
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}
	keepVendorInSync(".")
	warnStaleBridgePin(".")

	fmt.Printf("  Compiling Go binary for %s/%s...\n", target.GOOS, target.GOARCH)
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	absPath, _ := filepath.Abs(outputName)
	fmt.Printf("  Build complete: %s\n", absPath)
	return nil
}

func buildAndroidDev(deps *androidDeps) (string, error) {
	defer snapshotModFiles(".")() // keep go.mod/go.sum clean of x/mobile afterward
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(cwd, ".goleo", "android-dev")
	os.RemoveAll(buildDir)

	fmt.Println("  Resolving Go dependencies...")
	if err := ensureLocalReplace("."); err != nil {
		return "", fmt.Errorf("go module resolution: %w", err)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Stdout = os.Stdout
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		return "", fmt.Errorf("go mod tidy failed: %w", err)
	}

	fmt.Println("  Building Go mobile library with gomobile...")
	goGet := exec.Command("go", "get", "-tool", "golang.org/x/mobile/cmd/gobind")
	goGet.Env = modModEnv() // `go get` refuses to run in a vendored project's -mod=vendor
	goGet.Stdout = os.Stdout
	goGet.Stderr = os.Stderr
	if err := goGet.Run(); err != nil {
		fmt.Println("  Warning: could not add tool dependency:", err)
	}

	gomobileInit := exec.Command(deps.Gomobile, "init")
	gomobileInit.Stdout = os.Stdout
	gomobileInit.Stderr = os.Stderr
	setMobileEnv(gomobileInit, deps)
	if err := gomobileInit.Run(); err != nil {
		return "", fmt.Errorf("gomobile init failed: %w", err)
	}

	aanName := "goleo.aar"
	aanPath := filepath.Join(cwd, aanName)
	bindTags, err := mobileBindTags(".")
	if err != nil {
		return "", fmt.Errorf("detecting feature usage: %w", err)
	}
	bindTarget, err := androidBindTarget()
	if err != nil {
		return "", err
	}
	gomobileArgs := []string{
		"bind", "-v",
		"-tags", bindTags,
		"-o", aanPath,
		"-target", bindTarget,
		"-androidapi", fmt.Sprintf("%d", androidAPI),
		gomobilePkgDir(),
	}
	gomobile := exec.Command(deps.Gomobile, gomobileArgs...)
	gomobile.Stdout = os.Stdout
	gomobile.Stderr = os.Stderr
	setMobileEnv(gomobile, deps)
	if err := gomobile.Run(); err != nil {
		return "", fmt.Errorf("gomobile bind failed: %w", err)
	}
	defer os.Remove(aanPath)

	fmt.Println("  Generating dev Android project...")
	mobileCfg := loadMobileConfig(".")
	iconSrc, hasIcon := mobileIconSource()
	mobileCfg.HasIcon = hasIcon
	if err := extractMobileTemplate("android-dev", buildDir, &mobileCfg); err != nil {
		return "", fmt.Errorf("generating dev Android project: %w", err)
	}
	if hasIcon {
		resDir := filepath.Join(buildDir, "app", "src", "main", "res")
		if err := generateAndroidIcons(iconSrc, resDir); err != nil {
			fmt.Println("  Warning: could not generate launcher icons:", err)
		} else {
			fmt.Println("  Generated launcher icons from bundle.icon")
		}
	}

	libsDir := filepath.Join(buildDir, "app", "libs")
	os.MkdirAll(libsDir, 0755)
	if err := copyFile(aanPath, filepath.Join(libsDir, aanName)); err != nil {
		return "", fmt.Errorf("copying .aar: %w", err)
	}

	outputName := buildOutput
	if outputName == "" {
		outputName = "app-dev.apk"
	}
	outputPath := filepath.Join(cwd, outputName)

	fmt.Println("  Compiling dev APK with Gradle...")
	gradlew := filepath.Join(buildDir, "gradlew")
	if _, err := os.Stat(gradlew); os.IsNotExist(err) {
		if err := ensureGradleWrapper(buildDir); err != nil {
			return "", fmt.Errorf("preparing the Gradle wrapper: %w", err)
		}
	}

	gradleCmd := exec.Command(gradlew, "assembleDebug")
	gradleCmd.Dir = buildDir
	gradleCmd.Stdout = os.Stdout
	gradleCmd.Stderr = os.Stderr
	setMobileEnv(gradleCmd, deps)
	if err := gradleCmd.Run(); err != nil {
		return "", fmt.Errorf("gradle build failed: %w", err)
	}

	apkPath := filepath.Join(buildDir, "app", "build", "outputs", "apk", "debug", "app-debug.apk")
	if _, err := os.Stat(apkPath); err == nil {
		copyFile(apkPath, outputPath)
		fmt.Printf("  Dev APK: %s\n", outputPath)
	} else {
		return "", fmt.Errorf("APK not found at %s", apkPath)
	}

	fmt.Printf("  Dev Android build complete!\n")
	return outputPath, nil
}

func buildForAndroid(distDir string, deps *androidDeps) error {
	defer snapshotModFiles(".")() // keep go.mod/go.sum clean of x/mobile afterward
	fmt.Println("  Resolving Go dependencies...")
	if err := ensureLocalReplace("."); err != nil {
		return fmt.Errorf("go module resolution: %w", err)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Stdout = os.Stdout
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}

	cwd, _ := os.Getwd()
	buildDir := filepath.Join(cwd, ".goleo", "android")

	// Copy frontend dist into the gomobile package directory for embedding
	if distExists(distDir) {
		gmDist := filepath.Join(cwd, filepath.FromSlash(gomobilePkgDir()), "frontend", "dist")
		os.RemoveAll(gmDist)
		os.MkdirAll(filepath.Dir(gmDist), 0755)
		if err := copyDir(distDir, gmDist); err != nil {
			return fmt.Errorf("copying frontend dist for embed: %w", err)
		}
	}

	os.RemoveAll(buildDir)
	os.MkdirAll(buildDir, 0755)

	aanName := "goleo.aar"
	aanPath := filepath.Join(cwd, aanName)

	bindTags, err := mobileBindTags(".")
	if err != nil {
		return fmt.Errorf("detecting feature usage: %w", err)
	}

	bindTarget, err := androidBindTarget()
	if err != nil {
		return err
	}
	gomobileArgs := []string{
		"bind", "-v",
		"-tags", bindTags,
		"-o", aanPath,
		"-target", bindTarget,
		"-androidapi", fmt.Sprintf("%d", androidAPI),
		gomobilePkgDir(),
	}

	fmt.Println("  Adding golang.org/x/mobile tool dependency...")
	goGet := exec.Command("go", "get", "-tool", "golang.org/x/mobile/cmd/gobind")
	goGet.Env = modModEnv() // `go get` refuses to run in a vendored project's -mod=vendor
	goGet.Stdout = os.Stdout
	goGet.Stderr = os.Stderr
	if err := goGet.Run(); err != nil {
		fmt.Println("  Warning: could not add tool dependency:", err)
	}

	fmt.Println("  Initializing gomobile toolchain...")
	gomobileInit := exec.Command(deps.Gomobile, "init")
	gomobileInit.Stdout = os.Stdout
	gomobileInit.Stderr = os.Stderr
	setMobileEnv(gomobileInit, deps)
	if err := gomobileInit.Run(); err != nil {
		return fmt.Errorf("gomobile init failed: %w", err)
	}

	fmt.Println("  Building Go mobile library with gomobile...")
	gomobile := exec.Command(deps.Gomobile, gomobileArgs...)
	gomobile.Stdout = os.Stdout
	gomobile.Stderr = os.Stderr
	setMobileEnv(gomobile, deps)
	if err := gomobile.Run(); err != nil {
		return fmt.Errorf("gomobile bind failed: %w", err)
	}

	fmt.Println("  Generating Android project...")
	mobileCfg := loadMobileConfig(".")
	iconSrc, hasIcon := mobileIconSource()
	mobileCfg.HasIcon = hasIcon

	// versionCode precedence: the env var wins so CI can stamp a build number without
	// editing goleo.json, then --version-code, then goleo.json, then a value derived
	// from the semver. Play rejects an upload whose versionCode has not increased, and
	// it is the one field a human cannot reasonably remember to bump.
	if env := strings.TrimSpace(os.Getenv("GOLEO_ANDROID_VERSION_CODE")); env != "" {
		n, err := strconv.Atoi(env)
		if err != nil || n <= 0 {
			return fmt.Errorf("GOLEO_ANDROID_VERSION_CODE=%q is not a positive integer", env)
		}
		mobileCfg.VersionCode = n
	} else if buildVersionCode > 0 {
		mobileCfg.VersionCode = buildVersionCode
	}

	if err := setAndroidPermissions(&mobileCfg, "."); err != nil {
		return err
	}
	if err := extractMobileTemplate("android", buildDir, &mobileCfg); err != nil {
		return fmt.Errorf("generating Android project: %w", err)
	}
	if hasIcon {
		resDir := filepath.Join(buildDir, "app", "src", "main", "res")
		if err := generateAndroidIcons(iconSrc, resDir); err != nil {
			fmt.Println("  Warning: could not generate launcher icons:", err)
		} else {
			fmt.Println("  Generated launcher icons from bundle.icon")
		}
	}

	libsDir := filepath.Join(buildDir, "app", "libs")
	os.MkdirAll(libsDir, 0755)
	if err := copyFile(aanPath, filepath.Join(libsDir, aanName)); err != nil {
		return fmt.Errorf("copying .aar: %w", err)
	}

	if distExists(distDir) {
		assetsDir := filepath.Join(buildDir, "app", "src", "main", "assets")
		os.RemoveAll(assetsDir)
		if err := copyDir(distDir, assetsDir); err != nil {
			return fmt.Errorf("copying frontend assets: %w", err)
		}
	}

	format, err := resolveAndroidFormat(buildAndroidFormat, buildRelease)
	if err != nil {
		return err
	}

	// An unsigned release artifact is useless — Play rejects it and Android refuses to
	// install it — so this errors rather than following the "print a notice and carry
	// on" pattern the desktop signing paths use. --no-sign is the explicit way to say
	// you want the unsigned artifact anyway (to sign it yourself, or to check the
	// build works before setting up a keystore).
	signRelease := buildRelease && !buildNoSign
	if signRelease && !androidSigningConfigured() {
		return errAndroidKeystoreMissing
	}
	if buildRelease && buildNoSign {
		fmt.Println("  --no-sign: building an UNSIGNED release artifact (not installable or uploadable as-is)")
	}
	// Gradle reads the keystore itself from the environment (build.gradle.kts), so with
	// --no-sign the variable has to be taken away from it, or it would sign anyway.
	gradleEnvOverrides := androidSigningEnv()
	if buildRelease && buildNoSign {
		// Blank it last so it wins: Gradle reads the keystore itself, so --no-sign has to
		// take it away or Gradle would sign regardless.
		gradleEnvOverrides = append(gradleEnvOverrides, "GOLEO_ANDROID_KEYSTORE=")
	}

	outputName := buildOutput
	if outputName == "" {
		outputName = "app." + string(format)
	}
	outputPath := filepath.Join(cwd, outputName)

	gradleTask, artifactCandidates := androidGradleTask(format, buildRelease)
	kind := strings.ToUpper(string(format))
	fmt.Printf("  Compiling %s with Gradle (%s)...\n", kind, gradleTask)
	gradlew := filepath.Join(buildDir, "gradlew")
	if _, err := os.Stat(gradlew); os.IsNotExist(err) {
		// Report this. It was `_ = err` here while the dev-build path and
		// `goleo emulate` both return it — so a failed wrapper download (offline, a
		// proxy, GitHub down) fell through to a Gradle invocation that could only fail
		// more confusingly, or to the silent-success path below.
		if err := ensureGradleWrapper(buildDir); err != nil {
			return fmt.Errorf("preparing the Gradle wrapper: %w", err)
		}
	}

	gradleCmd := exec.Command(gradlew, gradleTask)
	gradleCmd.Dir = buildDir
	gradleCmd.Stdout = os.Stdout
	gradleCmd.Stderr = os.Stderr
	setMobileEnv(gradleCmd, deps)
	gradleCmd.Env = append(gradleCmd.Env, gradleEnvOverrides...)
	if err := gradleCmd.Run(); err != nil {
		return fmt.Errorf("gradle build failed: %w", err)
	}

	// Gradle exited 0, so the artifact must be where it says. If it is not, that is a
	// failure — this used to print "APK built in: <dir>" and return nil, so
	// `goleo build android` exited 0 having produced nothing at the path it just
	// told the user about. A CI job or a script checking the exit code saw success.
	artifact := ""
	for _, c := range artifactCandidates {
		p := filepath.Join(buildDir, c)
		if _, err := os.Stat(p); err == nil {
			artifact = p
			break
		}
	}
	if artifact == "" {
		return fmt.Errorf("gradle (%s) reported success but produced none of:\n    %s\n"+
			"  look under %s for what it did produce",
			gradleTask, strings.Join(artifactCandidates, "\n    "),
			filepath.Join(buildDir, "app", "build", "outputs"))
	}
	// A signed release must not come back as the -unsigned variant. Gradle silently
	// falls back to it when the signingConfig did not apply, so without this check
	// `--release` could hand over an unsigned artifact while reporting success.
	if signRelease && strings.Contains(filepath.Base(artifact), "-unsigned") {
		return fmt.Errorf("--release produced %s: the signing config did not apply.\n"+
			"  Check GOLEO_ANDROID_KEYSTORE_PASSWORD, GOLEO_ANDROID_KEY_ALIAS and\n"+
			"  GOLEO_ANDROID_KEY_PASSWORD — Gradle skips signing rather than failing when\n"+
			"  the keystore cannot be opened.", filepath.Base(artifact))
	}
	if err := copyFile(artifact, outputPath); err != nil {
		return fmt.Errorf("copying %s to %s: %w", filepath.Base(artifact), outputPath, err)
	}
	signedNote := ""
	if buildRelease {
		if signRelease {
			signedNote = " (signed release)"
		} else {
			signedNote = " (UNSIGNED release)"
		}
	}
	fmt.Printf("  %s: %s%s\n", kind, outputPath, signedNote)
	if format == androidFormatAAB && signRelease {
		fmt.Println("  Verify before uploading:  bundletool validate --bundle=" + outputName)
	}

	os.Remove(aanPath)
	fmt.Printf("  Android build complete!\n")
	return nil
}

// snapshotModFiles saves go.mod and go.sum in projectDir and returns a func that
// restores them. The mobile toolchain (`go get -tool …/gobind`, `gomobile bind`)
// adds build-only deps — golang.org/x/mobile and its tree — to go.mod under
// -mod=mod and never re-vendors. Restoring afterward keeps the project's module
// files clean and vendor-consistent, so a later desktop build (which runs
// -mod=vendor because the scaffold commits vendor/) doesn't fail with
// "inconsistent vendoring". Idiom: `defer snapshotModFiles(".")()` — the snapshot
// is taken immediately, the restore is deferred.
func snapshotModFiles(projectDir string) func() {
	saved := map[string][]byte{}
	for _, name := range []string{"go.mod", "go.sum"} {
		p := filepath.Join(projectDir, name)
		if data, err := os.ReadFile(p); err == nil {
			saved[p] = data
		}
	}
	return func() {
		for p, data := range saved {
			_ = os.WriteFile(p, data, 0o644)
		}
	}
}

func setMobileEnv(cmd *exec.Cmd, deps *androidDeps) {
	// Put the Go bin directory on PATH so gomobile can find gobind, which it
	// shells out to and which `go install` also drops into GOPATH/bin.
	env := prependPath(os.Environ(), goBinDir())
	if deps.JavaHome != "" {
		env = append(env, "JAVA_HOME="+deps.JavaHome)
	}
	if deps.SDKRoot != "" {
		env = append(env, "ANDROID_HOME="+deps.SDKRoot)
	}
	if deps.NDKDir != "" {
		env = append(env, "ANDROID_NDK_HOME="+deps.NDKDir)
	}
	// Force -mod=mod: gomobile bind needs golang.org/x/mobile's bind-support
	// packages, which a project's committed vendor/ does not contain (they are
	// only reached via gomobile's generated code). A vendored project must leave
	// vendor mode for the mobile toolchain to resolve them.
	env = upsertEnv(env, "GOFLAGS", "-mod=mod")
	cmd.Env = env
}

func buildForIOS(distDir string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("iOS builds require macOS with Xcode")
	}

	if err := checkCommand("gomobile", "golang.org/x/mobile/cmd/gomobile"); err != nil {
		return err
	}
	defer snapshotModFiles(".")() // keep go.mod/go.sum clean of x/mobile afterward

	fmt.Println("  Resolving Go dependencies...")
	if err := ensureLocalReplace("."); err != nil {
		return fmt.Errorf("go module resolution: %w", err)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Stdout = os.Stdout
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}

	cwd, _ := os.Getwd()
	buildDir := filepath.Join(cwd, ".goleo", "ios")

	// Copy frontend dist into the gomobile package directory for embedding
	if distExists(distDir) {
		gmDist := filepath.Join(cwd, filepath.FromSlash(gomobilePkgDir()), "frontend", "dist")
		os.RemoveAll(gmDist)
		os.MkdirAll(filepath.Dir(gmDist), 0755)
		if err := copyDir(distDir, gmDist); err != nil {
			return fmt.Errorf("copying frontend dist for embed: %w", err)
		}
	}

	os.RemoveAll(buildDir)
	os.MkdirAll(buildDir, 0755)

	xcfName := "goleo.xcframework"
	xcfPath := filepath.Join(cwd, xcfName)

	bindTags, err := mobileBindTags(".")
	if err != nil {
		return fmt.Errorf("detecting feature usage: %w", err)
	}

	gomobileArgs := []string{
		"bind", "-v",
		"-tags", bindTags,
		"-o", xcfPath,
		"-target", "ios",
		"-iosversion", iosDeployTarget,
		gomobilePkgDir(),
	}

	fmt.Println("  Adding golang.org/x/mobile tool dependency...")
	goGet := exec.Command("go", "get", "-tool", "golang.org/x/mobile/cmd/gobind")
	goGet.Env = modModEnv() // `go get` refuses to run in a vendored project's -mod=vendor
	goGet.Stdout = os.Stdout
	goGet.Stderr = os.Stderr
	if err := goGet.Run(); err != nil {
		fmt.Println("  Warning: could not add tool dependency:", err)
	}

	gomobilePath := "gomobile"
	if p, ok := findTool("gomobile"); ok {
		gomobilePath = p
	}

	fmt.Println("  Initializing gomobile toolchain...")
	gomobileInit := exec.Command(gomobilePath, "init")
	gomobileInit.Stdout = os.Stdout
	gomobileInit.Stderr = os.Stderr
	gomobileInit.Env = goToolEnv()
	if err := gomobileInit.Run(); err != nil {
		return fmt.Errorf("gomobile init failed: %w", err)
	}

	fmt.Println("  Building Go mobile library with gomobile...")
	gomobile := exec.Command(gomobilePath, gomobileArgs...)
	gomobile.Stdout = os.Stdout
	gomobile.Stderr = os.Stderr
	gomobile.Env = goToolEnv()
	if err := gomobile.Run(); err != nil {
		return fmt.Errorf("gomobile bind failed: %w", err)
	}

	fmt.Println("  Generating iOS project...")
	mobileCfg := loadMobileConfig(".")
	iconSrc, hasIcon := mobileIconSource()
	mobileCfg.HasIcon = hasIcon
	if err := extractMobileTemplate("ios", buildDir, &mobileCfg); err != nil {
		return fmt.Errorf("generating iOS project: %w", err)
	}
	if hasIcon {
		assetsDir := filepath.Join(buildDir, "App", "Assets.xcassets")
		if err := generateIOSAppIcon(iconSrc, assetsDir); err != nil {
			fmt.Println("  Warning: could not generate AppIcon set:", err)
		} else {
			fmt.Println("  Generated AppIcon.appiconset from bundle.icon")
		}
	}

	if err := copyDir(xcfPath, filepath.Join(buildDir, xcfName)); err != nil {
		return fmt.Errorf("copying .xcframework: %w", err)
	}

	if distExists(distDir) {
		appAssets := filepath.Join(buildDir, "App", "Assets")
		if err := copyDir(distDir, appAssets); err != nil {
			return fmt.Errorf("copying frontend assets: %w", err)
		}
	}

	outputName := buildOutput
	if outputName == "" {
		outputName = "GoleoApp.app"
	}
	outputPath := filepath.Join(cwd, outputName)

	fmt.Println("  Generating Xcode project with xcodegen...")
	if err := checkCommand("xcodegen", "xcodegen"); err != nil {
		return err
	}
	xcodegen := exec.Command("xcodegen", "--spec", filepath.Join(buildDir, "xcodegen.yml"))
	xcodegen.Dir = buildDir
	xcodegen.Stdout = os.Stdout
	xcodegen.Stderr = os.Stderr
	if err := xcodegen.Run(); err != nil {
		return fmt.Errorf("xcodegen failed: %w", err)
	}

	fmt.Println("  Compiling with xcodebuild...")
	xcodebuild := exec.Command("xcodebuild", "-project", filepath.Join(buildDir, "GoleoApp.xcodeproj"), "-scheme", "App", "-configuration", "Debug", "CONFIGURATION_BUILD_DIR="+cwd)
	xcodebuild.Stdout = os.Stdout
	xcodebuild.Stderr = os.Stderr
	if err := xcodebuild.Run(); err != nil {
		return fmt.Errorf("xcodebuild failed: %w", err)
	}

	os.RemoveAll(xcfPath)
	fmt.Printf("  iOS build complete: %s\n", outputPath)
	return nil
}

func buildForPWA(distDir string) error {
	// Verify frontend dist exists
	if !distExists(distDir) {
		return fmt.Errorf("frontend dist directory %s is empty or does not exist", distDir)
	}

	// Determine output directory
	outputDir := buildOutput
	if outputDir == "" {
		outputDir = "dist-pwa"
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Copy frontend dist files into output directory
	fmt.Println("  Copying frontend assets...")
	if err := copyDir(distDir, outputDir); err != nil {
		return fmt.Errorf("copying frontend assets: %w", err)
	}

	absPath, _ := filepath.Abs(outputDir)
	fmt.Printf("  PWA build complete: %s\n", absPath)
	return nil
}

// backendPkgDir returns the Go main-package directory: ./backend for the
// current project layout, "." for legacy projects with main.go at the root.
// Checks for the backend directory itself rather than backend/main.go,
// since main.go is generated fresh by generateBackendEntrypoints and may not
// exist yet on a fresh clone.
func backendPkgDir() string {
	if fi, err := os.Stat("backend"); err == nil && fi.IsDir() {
		return "./backend"
	}
	return "."
}

// gomobilePkgDir returns the gomobile bind package path, supporting both the
// backend/gomobile layout and the legacy root-level gomobile package.
func gomobilePkgDir() string {
	if fi, err := os.Stat(filepath.Join("backend", "gomobile")); err == nil && fi.IsDir() {
		return "./backend/gomobile"
	}
	return "./gomobile"
}

func distExists(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && e.Name() != ".gitkeep" {
			return true
		}
	}
	return false
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	os.MkdirAll(dst, 0755)
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureGradleWrapper writes the wrapper scripts and the VENDORED wrapper JAR.
//
// This used to download the JAR with http.Get. Two problems with that, both real:
// the default http.Client has NO timeout, so a hung connection hung the build
// indefinitely with no output; and there was no integrity check of any kind on a JAR
// that is then executed via `java -classpath`. Anyone able to interpose on that request
// could run code on a developer's machine and in their CI.
//
// The JAR is now committed under cli/cmd/gradlewrapper (see the README there for
// provenance and how to update it) and written from the embed, so the build touches the
// network for this at all. A pinned checksum was the alternative, but that means owning a
// checksum treadmill for every Gradle bump; vendoring matches what this repo already does
// for Go dependencies.
func ensureGradleWrapper(dir string) error {
	jarDir := filepath.Join(dir, "gradle", "wrapper")
	if err := os.MkdirAll(jarDir, 0755); err != nil {
		return fmt.Errorf("creating wrapper dir: %w", err)
	}

	jarPath := filepath.Join(jarDir, "gradle-wrapper.jar")
	if _, err := os.Stat(jarPath); err == nil {
		return nil
	}

	batScript := filepath.Join(dir, "gradlew.bat")
	batContent := `@echo off
set DIRNAME=%~dp0
if "%DIRNAME%" == "" set DIRNAME=.
"%JAVA_HOME%/bin/java" -Dorg.gradle.appname=gradlew -classpath "%DIRNAME%/gradle/wrapper/gradle-wrapper.jar" org.gradle.wrapper.GradleWrapperMain %*
`
	if err := os.WriteFile(batScript, []byte(batContent), 0755); err != nil {
		return fmt.Errorf("writing gradlew.bat: %w", err)
	}

	shScript := filepath.Join(dir, "gradlew")
	shContent := `#!/bin/sh
DIRNAME="$(dirname "$0")"
java -Dorg.gradle.appname=gradlew -classpath "$DIRNAME/gradle/wrapper/gradle-wrapper.jar" org.gradle.wrapper.GradleWrapperMain "$@"
`
	if err := os.WriteFile(shScript, []byte(shContent), 0755); err != nil {
		return fmt.Errorf("writing gradlew: %w", err)
	}

	// Sanity-check the embedded JAR before handing it to java. Cheap, and it turns a
	// corrupted or truncated embed into a clear error here rather than an obscure
	// failure inside Gradle's bootstrap.
	if err := checkWrapperJar(gradleWrapperJAR); err != nil {
		return fmt.Errorf("vendored gradle-wrapper.jar is unusable: %w", err)
	}
	if err := os.WriteFile(jarPath, gradleWrapperJAR, 0644); err != nil {
		return fmt.Errorf("writing gradle-wrapper.jar: %w", err)
	}
	return nil
}

// checkWrapperJar verifies the bytes are a JAR containing the wrapper entry point.
//
// Structural, not a checksum: the point is to catch a broken or empty embed, not to
// authenticate the file — provenance is handled by it being committed and reviewed.
func checkWrapperJar(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("not a valid JAR/zip: %w", err)
	}
	const entry = "org/gradle/wrapper/GradleWrapperMain.class"
	for _, f := range zr.File {
		if f.Name == entry {
			return nil
		}
	}
	return fmt.Errorf("missing %s (%d entries) — not a Gradle wrapper JAR", entry, len(zr.File))
}

// checkGoleoJSON is the single validation gate for goleo.json. It used to only
// Stat the file, while every loader swallowed parse errors and fell back to
// defaults — so a trailing comma produced a successfully-built app carrying the
// wrong applicationId, bundle identifier and version, silently. Parse it here and
// fail loudly instead; the loaders stay tolerant because this already ran.
func checkGoleoJSON() error {
	cfg, found, err := parseGoleoJSON(".")
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("goleo.json not found: are you in a Goleo project directory?")
	}
	return validateGoleoJSON(cfg)
}

// validateGoleoJSON catches values that parse as JSON but cannot produce a
// correct build, so they surface now rather than as a store rejection later.
func validateGoleoJSON(cfg goleoJSON) error {
	if pkg := cfg.Mobile.Android.PackageName; pkg != "" && !strings.Contains(pkg, ".") {
		return fmt.Errorf("goleo.json: mobile.android.package_name %q must be reverse-DNS (e.g. com.example.app)", pkg)
	}
	if id := cfg.Mobile.IOS.BundleIdentifier; id != "" && !strings.Contains(id, ".") {
		return fmt.Errorf("goleo.json: mobile.ios.bundle_identifier %q must be reverse-DNS (e.g. com.example.app)", id)
	}
	if v := cfg.Mobile.Android.VersionCode; v < 0 {
		return fmt.Errorf("goleo.json: mobile.android.version_code must be positive, got %d", v)
	}
	if s := cfg.Mobile.Android.MinSDK; s != 0 && (s < 21 || s > 99) {
		return fmt.Errorf("goleo.json: mobile.android.min_sdk %d is out of range", s)
	}
	return nil
}

func checkCommand(name, installHint string) error {
	if _, ok := findTool(name); !ok {
		return fmt.Errorf("%s not found. Install it with: go install %s@latest", name, installHint)
	}
	return nil
}

// knownGoArches are the architectures --arch accepts for desktop targets.
var knownGoArches = []string{"amd64", "arm64", "386", "arm"}

func validGoArch(a string) bool {
	for _, k := range knownGoArches {
		if a == k {
			return true
		}
	}
	return false
}

// androidABIToGOARCH maps Android ABI names onto the GOARCH values gomobile's
// -target actually accepts.
//
// The two vocabularies differ, and the one users have to hand is the ABI name:
// that is what `adb shell getprop ro.product.cpu.abi` prints, what Play
// Console shows, and what an APK's lib/ directories are named. gomobile wants
// GOARCH. Passing an ABI name straight through fails with
// `unsupported platform/arch: "android/x86_64"`, so accept both spellings and
// translate.
var androidABIToGOARCH = map[string]string{
	// ABI name -> GOARCH
	"arm64-v8a":   "arm64",
	"armeabi-v7a": "arm",
	"x86_64":      "amd64",
	"x86":         "386",
	// GOARCH passed through unchanged, for callers who already speak Go's names.
	"arm64": "arm64",
	"arm":   "arm",
	"amd64": "amd64",
	"386":   "386",
}

// androidABIChoices lists the accepted --android-abi values for error messages,
// ABI names first since those are the ones users are likely to have.
var androidABIChoices = []string{"arm64-v8a", "armeabi-v7a", "x86_64", "x86", "arm64", "arm", "amd64", "386"}

// androidBindTarget renders gomobile's -target for the Android bind.
//
// Plain "android" builds every supported ABI — arm64-v8a, armeabi-v7a, x86 and
// x86_64 — which is right for a Play upload but quadruples the APK for anyone
// installing on one device: an emulator only needs x86_64, and a phone only
// needs arm64-v8a. Oversized APKs also fail to install on a nearly-full device,
// which Android reports as INSTALL_FAILED_INSUFFICIENT_STORAGE.
func androidBindTarget() (string, error) {
	if buildAndroidABI == "" {
		return "android", nil
	}
	targets := make([]string, 0, 4)
	seen := make(map[string]bool, 4)
	for _, part := range strings.Split(buildAndroidABI, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		arch, ok := androidABIToGOARCH[part]
		if !ok {
			return "", fmt.Errorf("unsupported --android-abi %q (want one of: %s)", part, strings.Join(androidABIChoices, ", "))
		}
		if seen[arch] {
			continue // e.g. "x86_64,amd64" — the same target spelled twice
		}
		seen[arch] = true
		targets = append(targets, "android/"+arch)
	}
	if len(targets) == 0 {
		return "android", nil
	}
	return strings.Join(targets, ","), nil
}

// gradleWrapperVersion is the Gradle version the Android template targets. It is the
// single source of truth for BOTH the vendored wrapper jar (cli/cmd/gradlewrapper)
// and the distributionUrl in templates/android/gradle/wrapper/gradle-wrapper.properties;
// a test asserts they agree, because they had silently drifted (jar 8.10.2 against a
// 9.4.1 distribution).
const gradleWrapperVersion = "9.4.1"

// validateAndroidRelease checks the release/signing flags before any expensive work.
//
// buildForAndroid re-resolves the format where it needs it; this exists purely so the
// failure arrives in the first second rather than after a gomobile bind.
func validateAndroidRelease() error {
	if _, err := resolveAndroidFormat(buildAndroidFormat, buildRelease); err != nil {
		return err
	}
	if buildRelease && !buildNoSign {
		if !androidSigningConfigured() {
			return errAndroidKeystoreMissing
		}
		// Check the keystore is actually there. Without this a typo, a relative path from
		// the wrong directory, or a stray trailing space failed inside Gradle at
		// :app:packageRelease after a full build — reported as
		// "Trailing char < > at index 141", which names neither the variable nor the
		// space. `set VAR=path ` in cmd.exe keeps that space, and Windows is where these
		// get set by hand.
		ks := strings.TrimSpace(os.Getenv("GOLEO_ANDROID_KEYSTORE"))
		if _, err := os.Stat(ks); err != nil {
			hint := ""
			if raw := os.Getenv("GOLEO_ANDROID_KEYSTORE"); raw != ks {
				hint = "\n  NOTE: the variable has leading or trailing whitespace — quote the whole\n" +
					`  assignment, e.g. set "GOLEO_ANDROID_KEYSTORE=C:\path\to\release.jks"`
			}
			return fmt.Errorf("GOLEO_ANDROID_KEYSTORE points at %q, which cannot be opened: %w%s\n"+
				"  Use an absolute path, or create one with `goleo generate android-key`.", ks, err, hint)
		}
	}
	return nil
}

// androidSigningEnv returns TRIMMED signing variables to hand to Gradle.
//
// The values are re-exported rather than inherited so Gradle always sees clean ones: the
// build.gradle.kts template trims defensively too, but a value that is wrong before it
// gets there is better fixed once, here, than relied upon to be tolerated downstream.
func androidSigningEnv() []string {
	var out []string
	for _, k := range []string{
		"GOLEO_ANDROID_KEYSTORE",
		"GOLEO_ANDROID_KEYSTORE_PASSWORD",
		"GOLEO_ANDROID_KEY_ALIAS",
		"GOLEO_ANDROID_KEY_PASSWORD",
	} {
		if raw, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+strings.TrimSpace(raw))
		}
	}
	return out
}

// errAndroidKeystoreMissing is returned when --release has no signing configuration.
//
// A sentinel rather than two copies of the message: it is raised both by the early
// validation in runBuild and by buildForAndroid, which keeps its own check so the
// function stays correct if ever called from somewhere that skips validation.
var errAndroidKeystoreMissing = fmt.Errorf(
	"--release needs a keystore: set GOLEO_ANDROID_KEYSTORE (plus\n" +
		"  GOLEO_ANDROID_KEYSTORE_PASSWORD, GOLEO_ANDROID_KEY_ALIAS, GOLEO_ANDROID_KEY_PASSWORD).\n" +
		"  Generate one with:\n" +
		"    keytool -genkeypair -v -keystore release.jks -keyalg RSA -keysize 2048 \\n" +
		"      -validity 10000 -alias upload\n" +
		"  Keep it and its passwords safe: losing the upload key means you cannot ship an\n" +
		"  update to an existing Play listing. Or pass --no-sign to build unsigned.")

// gradleWrapperJAR is the vendored Gradle wrapper, written into a generated Android
// project by ensureGradleWrapper. Committed rather than downloaded — see
// cli/cmd/gradlewrapper/README.md for why, its provenance, and how to update it.
//
//go:embed gradlewrapper/gradle-wrapper.jar
var gradleWrapperJAR []byte
