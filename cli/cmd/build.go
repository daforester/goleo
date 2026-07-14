package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	buildOutput     string
	buildFrontend   string
	buildAndroid    string
	androidAPI      int
	iosDeployTarget string
	buildBundle     bool
	buildPublish    bool
)

func init() {
	buildCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "Output file name")
	buildCmd.Flags().StringVarP(&buildFrontend, "frontend-dir", "f", "frontend", "Path to frontend directory")
	buildCmd.Flags().StringVarP(&buildAndroid, "android-ndk", "", "", "Path to Android NDK")
	buildCmd.Flags().IntVarP(&androidAPI, "android-api", "", 24, "Android API level")
	buildCmd.Flags().StringVarP(&iosDeployTarget, "ios-target", "", "14.0", "iOS deployment target")
	buildCmd.Flags().BoolVar(&buildBundle, "bundle", false, "Package the built desktop app into a native installer (dist/bundle/)")
	buildCmd.Flags().BoolVar(&buildPublish, "publish", false, "Write an ed25519-signed update manifest for the built binary (needs GOLEO_UPDATE_PRIVKEY)")
}

type buildTarget struct {
	GOOS      string
	GOARCH    string
	OutputExt string
	Label     string
}

var targets = map[string]buildTarget{
	"current": {GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, OutputExt: "", Label: "current"},
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
	if !ok {
		return fmt.Errorf("unknown target: %s\nAvailable: current, windows, linux, darwin, android, ios, pwa", targetName)
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

	frontendDist := filepath.Join(buildFrontend, "dist")
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
		outName := buildOutput
		if outName == "" {
			outName = "app"
		}
		binPath, _ := filepath.Abs(outName + target.OutputExt)
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

	fmt.Println("  Building frontend with Vite...")
	viteBuild := exec.Command("npx", "vite", "build")
	viteBuild.Dir = frontendDir
	viteBuild.Env = append(os.Environ(), extraEnv...)
	viteBuild.Stdout = os.Stdout
	viteBuild.Stderr = os.Stderr
	if err := viteBuild.Run(); err != nil {
		return fmt.Errorf("vite build failed: %w", err)
	}

	return nil
}

func buildForDesktop(target buildTarget, distDir string) error {
	outputName := buildOutput
	if outputName == "" {
		outputName = "app"
	}

	env := os.Environ()
	env = append(env, fmt.Sprintf("GOOS=%s", target.GOOS))
	env = append(env, fmt.Sprintf("GOARCH=%s", target.GOARCH))
	// All desktop targets are cgo-free by default via the purego glaze backend
	// (runtime/webview_glaze.go) — WKWebView (macOS), WebKitGTK (Linux) and
	// WebView2 (Windows) behind one binding. So every desktop build is
	// CGO_ENABLED=0 and cross-compiles from any host. One opt-in fallback remains
	// (one release, then removed): GOLEO_CGO_WEBVIEW=1 puts macOS/Linux back on the
	// cgo webview_go backend (needs CGO_ENABLED=1 + its own-OS toolchain).
	cgoWebview := os.Getenv("GOLEO_CGO_WEBVIEW") == "1" &&
		(target.GOOS == "darwin" || target.GOOS == "linux")
	if cgoWebview {
		env = append(env, "CGO_ENABLED=1")
	} else {
		env = append(env, "CGO_ENABLED=0")
	}

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
	if cgoWebview {
		args = append(args, "-tags", "goleo_cgo_webview")
	}
	args = append(args, "-o", outputName+target.OutputExt, pkgDir)

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

	fmt.Printf("  Compiling Go binary for %s/%s...\n", target.GOOS, target.GOARCH)
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	absPath, _ := filepath.Abs(outputName + target.OutputExt)
	fmt.Printf("  Build complete: %s\n", absPath)
	return nil
}

func buildAndroidDev(deps *androidDeps) (string, error) {
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
	gomobileArgs := []string{
		"bind", "-v",
		"-tags", bindTags,
		"-o", aanPath,
		"-target", "android",
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
	if err := extractMobileTemplate("android-dev", buildDir, &mobileCfg); err != nil {
		return "", fmt.Errorf("generating dev Android project: %w", err)
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
		if err := downloadGradleWrapper(buildDir); err != nil {
			return "", fmt.Errorf("downloading Gradle wrapper: %w", err)
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

	gomobileArgs := []string{
		"bind", "-v",
		"-tags", bindTags,
		"-o", aanPath,
		"-target", "android",
		"-androidapi", fmt.Sprintf("%d", androidAPI),
		gomobilePkgDir(),
	}

	fmt.Println("  Adding golang.org/x/mobile tool dependency...")
	goGet := exec.Command("go", "get", "-tool", "golang.org/x/mobile/cmd/gobind")
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
	if err := extractMobileTemplate("android", buildDir, &mobileCfg); err != nil {
		return fmt.Errorf("generating Android project: %w", err)
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

	outputName := buildOutput
	if outputName == "" {
		outputName = "app.apk"
	}
	outputPath := filepath.Join(cwd, outputName)

	fmt.Println("  Compiling APK with Gradle...")
	gradlew := filepath.Join(buildDir, "gradlew")
	if _, err := os.Stat(gradlew); os.IsNotExist(err) {
		if err := downloadGradleWrapper(buildDir); err != nil {
			_ = err
		}
	}

	gradleCmd := exec.Command(gradlew, "assembleDebug")
	gradleCmd.Dir = buildDir
	gradleCmd.Stdout = os.Stdout
	gradleCmd.Stderr = os.Stderr
	setMobileEnv(gradleCmd, deps)
	if err := gradleCmd.Run(); err != nil {
		return fmt.Errorf("gradle build failed: %w", err)
	}

	apkPath := filepath.Join(buildDir, "app", "build", "outputs", "apk", "debug", "app-debug.apk")
	if _, err := os.Stat(apkPath); err == nil {
		copyFile(apkPath, outputPath)
		fmt.Printf("  APK: %s\n", outputPath)
	} else {
		fmt.Println("  APK built in:", filepath.Join(buildDir, "app", "build", "outputs", "apk"))
	}

	os.Remove(aanPath)
	fmt.Printf("  Android build complete!\n")
	return nil
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
	cmd.Env = env
}

func buildForIOS(distDir string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("iOS builds require macOS with Xcode")
	}

	if err := checkCommand("gomobile", "golang.org/x/mobile/cmd/gomobile"); err != nil {
		return err
	}

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
	if err := extractMobileTemplate("ios", buildDir, &mobileCfg); err != nil {
		return fmt.Errorf("generating iOS project: %w", err)
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

func downloadGradleWrapper(dir string) error {
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
	os.WriteFile(batScript, []byte(batContent), 0755)

	shScript := filepath.Join(dir, "gradlew")
	shContent := `#!/bin/sh
DIRNAME="$(dirname "$0")"
java -Dorg.gradle.appname=gradlew -classpath "$DIRNAME/gradle/wrapper/gradle-wrapper.jar" org.gradle.wrapper.GradleWrapperMain "$@"
`
	os.WriteFile(shScript, []byte(shContent), 0755)

	jarURL := "https://github.com/gradle/gradle/raw/v8.10.2/gradle/wrapper/gradle-wrapper.jar"
	fmt.Println("  Downloading Gradle wrapper JAR...")
	resp, err := http.Get(jarURL)
	if err != nil {
		return fmt.Errorf("downloading wrapper JAR: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d downloading wrapper JAR", resp.StatusCode)
	}

	out, err := os.Create(jarPath)
	if err != nil {
		return fmt.Errorf("creating wrapper JAR: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("writing wrapper JAR: %w", err)
	}

	return nil
}

func checkGoleoJSON() error {
	if _, err := os.Stat("goleo.json"); os.IsNotExist(err) {
		return fmt.Errorf("goleo.json not found: are you in a Goleo project directory?")
	}
	return nil
}

func checkCommand(name, installHint string) error {
	if _, ok := findTool(name); !ok {
		return fmt.Errorf("%s not found. Install it with: go install %s@latest", name, installHint)
	}
	return nil
}
