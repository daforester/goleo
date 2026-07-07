package cmd

import (
	"fmt"
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
	buildOutput    string
	buildFrontend  string
	buildAndroid   string
	androidAPI     int
	iosDeployTarget string
)

func init() {
	buildCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "Output file name")
	buildCmd.Flags().StringVarP(&buildFrontend, "frontend-dir", "f", "frontend", "Path to frontend directory")
	buildCmd.Flags().StringVarP(&buildAndroid, "android-ndk", "", "", "Path to Android NDK")
	buildCmd.Flags().IntVarP(&androidAPI, "android-api", "", 24, "Android API level")
	buildCmd.Flags().StringVarP(&iosDeployTarget, "ios-target", "", "14.0", "iOS deployment target")
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
}

func runBuild(cmd *cobra.Command, args []string) error {
	targetName := "current"
	if len(args) > 0 {
		targetName = strings.ToLower(args[0])
	}

	target, ok := targets[targetName]
	if !ok {
		return fmt.Errorf("unknown target: %s\nAvailable: current, windows, linux, darwin, android, ios", targetName)
	}

	if err := checkGoleoJSON(); err != nil {
		return err
	}

	fmt.Printf("  Building Goleo app for %s (%s/%s)...\n", target.Label, target.GOOS, target.GOARCH)
	fmt.Println()

	frontendDist := filepath.Join(buildFrontend, "dist")
	if err := buildFrontendProject(buildFrontend, frontendDist); err != nil {
		return fmt.Errorf("frontend build failed: %w", err)
	}

	if targetName == "android" {
		return buildForAndroid(frontendDist)
	}
	if targetName == "ios" {
		return buildForIOS(frontendDist)
	}

	return buildForDesktop(target, frontendDist)
}

func buildFrontendProject(frontendDir, distDir string) error {
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
	env = append(env, "CGO_ENABLED=0")

	ldflags := fmt.Sprintf("-s -w -X main.Version=%s", "0.1.0")

	args := []string{"build", "-ldflags", ldflags, "-o", outputName + target.OutputExt, "."}

	build := exec.Command("go", args...)
	build.Env = env
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr

	fmt.Printf("  Compiling Go binary for %s/%s...\n", target.GOOS, target.GOARCH)
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	absPath, _ := filepath.Abs(outputName + target.OutputExt)
	fmt.Printf("  Build complete: %s\n", absPath)
	return nil
}

func buildForAndroid(distDir string) error {
	if err := checkCommand("gomobile", "gomobile"); err != nil {
		return err
	}

	fmt.Println("  Building Android package with gomobile...")

	env := os.Environ()
	if buildAndroid != "" {
		env = append(env, fmt.Sprintf("ANDROID_NDK_HOME=%s", buildAndroid))
	}

	outputName := buildOutput
	if outputName == "" {
		outputName = "app.aar"
	}

	args := []string{
		"bind",
		"-v",
		"-o", outputName,
		"-target", "android",
		"-androidapi", fmt.Sprintf("%d", androidAPI),
		"./backend",
	}

	build := exec.Command("gomobile", args...)
	build.Env = append(os.Environ(), env...)
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr

	if err := build.Run(); err != nil {
		return fmt.Errorf("gomobile build failed: %w", err)
	}

	absPath, _ := filepath.Abs(outputName)
	fmt.Printf("  Android build complete: %s\n", absPath)
	fmt.Println("  Import the .aar into your Android project or use gomobile build for .apk")
	return nil
}

func buildForIOS(distDir string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("iOS builds require macOS with Xcode")
	}

	if err := checkCommand("gomobile", "gomobile"); err != nil {
		return err
	}

	fmt.Println("  Building iOS package with gomobile...")

	outputName := buildOutput
	if outputName == "" {
		outputName = "app.xcframework"
	}

	args := []string{
		"bind",
		"-v",
		"-o", outputName,
		"-target", "ios",
		"-iosversion", iosDeployTarget,
		"./backend",
	}

	build := exec.Command("gomobile", args...)
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr

	if err := build.Run(); err != nil {
		return fmt.Errorf("gomobile iOS build failed: %w", err)
	}

	absPath, _ := filepath.Abs(outputName)
	fmt.Printf("  iOS build complete: %s\n", absPath)
	fmt.Println("  Import the .xcframework into your Xcode project")
	return nil
}

func checkGoleoJSON() error {
	if _, err := os.Stat("goleo.json"); os.IsNotExist(err) {
		return fmt.Errorf("goleo.json not found: are you in a Goleo project directory?")
	}
	return nil
}

func checkCommand(name, installHint string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s not found. Install it with: go install %s@latest", name, installHint)
	}
	return nil
}
