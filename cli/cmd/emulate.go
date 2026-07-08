package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

var emulateCmd = &cobra.Command{
	Use:   "emulate [target]",
	Short: "Run the app on an emulator (Android only for now)",
	Long: `Run the application on an emulator or connected device.

In dev mode, the Go backend runs inside the emulator via gomobile AAR.
The frontend Vite dev server runs on the host with HMR.
The emulator WebView loads from 10.0.2.2 and connects to localhost:9842.

Go source changes trigger automatic rebuild and redeploy.

Targets:
  android    Run in dev mode on Android emulator`,
	Args: cobra.ExactArgs(1),
	RunE: runEmulate,
}

var (
	emulateTarget   string
	emulateHeadless bool
)

func init() {
	emulateCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "Output APK name")
	emulateCmd.Flags().StringVarP(&buildAndroid, "android-ndk", "", "", "Path to Android NDK")
	emulateCmd.Flags().IntVarP(&devPort, "port", "p", 9842, "Port for the Go backend server")
	emulateCmd.Flags().BoolVar(&emulateHeadless, "headless", false, "Start the emulator without a window (for CI)")
}

func runEmulate(cmd *cobra.Command, args []string) error {
	target := strings.ToLower(args[0])

	switch target {
	case "android":
		return emulateAndroid()
	default:
		return fmt.Errorf("unsupported target: %s. Supported: android", target)
	}
}

func emulateAndroid() error {
	if err := checkGoleoJSON(); err != nil {
		return err
	}

	deps, err := ensureAndroidDeps()
	if err != nil {
		return err
	}

	if deps.AdbPath == "" {
		return fmt.Errorf("adb not found: install Android platform-tools")
	}

	cwd, _ := os.Getwd()
	frontendAbs := filepath.Join(cwd, "frontend")
	pkgName := "com.goleo.app"
	if data, err := os.ReadFile("goleo.json"); err == nil {
		pkgName = extractPackageName(string(data))
	}

	// 1. Start Vite dev server on host
	fmt.Println("  Starting Vite frontend...")
	vitePort := 5173
	viteCmd := exec.Command("npx", "vite", "--port", fmt.Sprintf("%d", vitePort), "--host")
	viteCmd.Dir = frontendAbs
	viteCmd.Stdout = os.Stdout
	viteCmd.Stderr = os.Stderr
	if err := viteCmd.Start(); err != nil {
		return fmt.Errorf("failed to start Vite: %w", err)
	}
	defer killProcTree(viteCmd.Process.Pid)

	// Wait for Vite ready
	fmt.Println("  Waiting for Vite...")
	for i := 0; i < 30; i++ {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", vitePort), time.Second)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("  Vite ready.")

	// 2. Find or start emulator
	deviceID, err := findDevice(deps.AdbPath, deps.EmulatorPath)
	if err != nil {
		return err
	}
	fmt.Printf("  Found device: %s\n", deviceID)

	if !isBootCompleted(deps.AdbPath, deviceID) {
		fmt.Println("  Waiting for system to boot...")
		deadline := time.Now().Add(3 * time.Minute)
		for time.Now().Before(deadline) {
			if isBootCompleted(deps.AdbPath, deviceID) {
				break
			}
			time.Sleep(3 * time.Second)
			fmt.Print(".")
		}
		fmt.Println()
	}

	// 3. Build and deploy (initial)
	if err := buildAndDeployDev(deps, deviceID, pkgName); err != nil {
		return err
	}

	// 4. Start file watcher for Go source
	ctx, stopWatcher := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopWatcher()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("file watcher: %w", err)
	}
	defer watcher.Close()

	for _, dir := range []string{
		cwd,
		filepath.Join(cwd, "backend"),
		filepath.Join(cwd, "backend", "commands"),
		filepath.Join(cwd, "backend", "gomobile"),
		filepath.Join(cwd, "gomobile"),
	} {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			watcher.Add(dir)
		}
	}

	var (
		debounce *time.Timer
		buildMu  sync.Mutex
	)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !strings.HasSuffix(event.Name, ".go") {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
					continue
				}
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(500*time.Millisecond, func() {
					if !buildMu.TryLock() {
						return
					}
					defer buildMu.Unlock()

					fmt.Println("\n  Go source changed, rebuilding & redeploying...")
					if err := buildAndDeployDev(deps, deviceID, pkgName); err != nil {
						fmt.Printf("  Rebuild failed: %v\n", err)
					}
				})
			case err, ok := <-watcher.Errors:
				if ok {
					fmt.Printf("  Watcher error: %v\n", err)
				}
			}
		}
	}()

	fmt.Println()
	fmt.Printf("  Dev server running. Frontend: http://localhost:%d\n", vitePort)
	fmt.Println("  Go backend runs inside the emulator on port 9842")
	fmt.Println("  Watching Go source for changes (Ctrl+C to stop)...")
	fmt.Println()

	// 5. Wait for Ctrl+C
	<-ctx.Done()
	fmt.Println("\n  Shutting down...")
	return nil
}

func buildAndDeployDev(deps *androidDeps, deviceID, pkgName string) error {
	cwd, _ := os.Getwd()
	buildDir := filepath.Join(cwd, ".goleo", "android-dev")

	// Build AAR
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

	fmt.Println("  Building Go mobile library with gomobile...")
	goGet := exec.Command("go", "get", "-tool", "golang.org/x/mobile/cmd/gobind")
	goGet.Stdout = os.Stdout
	goGet.Stderr = os.Stderr
	goGet.Run()

	gomobileInit := exec.Command(deps.Gomobile, "init")
	gomobileInit.Stdout = os.Stdout
	gomobileInit.Stderr = os.Stderr
	setMobileEnv(gomobileInit, deps)
	if err := gomobileInit.Run(); err != nil {
		return fmt.Errorf("gomobile init failed: %w", err)
	}

	aanName := "goleo.aar"
	aanPath := filepath.Join(cwd, aanName)
	gomobileArgs := []string{
		"bind", "-v",
		"-tags", "mobilebuild,goleodev",
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
		os.Remove(aanPath)
		return fmt.Errorf("gomobile bind failed: %w", err)
	}
	defer os.Remove(aanPath)

	// Generate Android project
	os.RemoveAll(buildDir)
	mobileCfg := loadMobileConfig(".")
	if err := extractMobileTemplate("android-dev", buildDir, &mobileCfg); err != nil {
		return fmt.Errorf("generating dev Android project: %w", err)
	}

	libsDir := filepath.Join(buildDir, "app", "libs")
	os.MkdirAll(libsDir, 0755)
	if err := copyFile(aanPath, filepath.Join(libsDir, aanName)); err != nil {
		return fmt.Errorf("copying .aar: %w", err)
	}

	// Build APK
	fmt.Println("  Compiling dev APK with Gradle...")
	gradlew := filepath.Join(buildDir, "gradlew")
	if _, err := os.Stat(gradlew); os.IsNotExist(err) {
		if err := downloadGradleWrapper(buildDir); err != nil {
			return fmt.Errorf("downloading Gradle wrapper: %w", err)
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
	if _, err := os.Stat(apkPath); err != nil {
		return fmt.Errorf("APK not found at %s", apkPath)
	}

	// Install
	fmt.Println("  Installing dev APK...")
	uninstall := exec.Command(deps.AdbPath, "-s", deviceID, "uninstall", pkgName)
	uninstall.Run()

	install := exec.Command(deps.AdbPath, "-s", deviceID, "install", "-r", "-d", apkPath)
	install.Stdout = os.Stdout
	install.Stderr = os.Stderr
	if err := install.Run(); err != nil {
		return fmt.Errorf("adb install failed: %w", err)
	}

	// Launch
	fmt.Println("  Launching app...")
	launch := exec.Command(deps.AdbPath, "-s", deviceID, "shell", "am", "start", "-n", pkgName+"/.MainActivity")
	launch.Stdout = os.Stdout
	launch.Stderr = os.Stderr
	if err := launch.Run(); err != nil {
		fmt.Printf("  Warning: could not launch app: %v\n", err)
		fmt.Println("  Open the app manually in the emulator.")
	} else {
		fmt.Printf("  App launched on %s!\n", deviceID)
	}

	return nil
}

func findDevice(adbPath, emulatorPath string) (string, error) {
	// 1. Check for already-running devices
	if deviceID := findRunningDevice(adbPath); deviceID != "" {
		return deviceID, nil
	}

	// 2. No device found, try to start an emulator
	if emulatorPath == "" {
		return "", fmt.Errorf("no device found and emulator binary not found. Start an emulator or connect a device via USB.")
	}

	avds, err := listAVDs(emulatorPath)
	if err != nil || len(avds) == 0 {
		return "", fmt.Errorf("no device found and no AVDs available. Create an AVD in Android Studio first.")
	}

	avdName := avds[0]
	fmt.Printf("  Starting emulator: %s\n", avdName)

	emuArgs := []string{"-avd", avdName, "-no-snapshot-load"}
	if emulateHeadless {
		emuArgs = append(emuArgs, "-no-audio", "-no-window")
	}
	emu := exec.Command(emulatorPath, emuArgs...)
	if err := emu.Start(); err != nil {
		return "", fmt.Errorf("failed to start emulator: %w", err)
	}

	// 3. Wait for the emulator to appear in adb and finish booting
	fmt.Println("  Waiting for emulator to boot...")
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		deviceID := findRunningDevice(adbPath)
		if deviceID != "" && isBootCompleted(adbPath, deviceID) {
			fmt.Println()
			fmt.Println("  Emulator is ready.")
			return deviceID, nil
		}
		time.Sleep(3 * time.Second)
		fmt.Print(".")
	}
	fmt.Println()
	return "", fmt.Errorf("timed out waiting for emulator to boot")
}

func findRunningDevice(adbPath string) string {
	out, err := exec.Command(adbPath, "devices").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == "device" {
			return parts[0]
		}
	}
	return ""
}

func isBootCompleted(adbPath, deviceID string) bool {
	out, err := exec.Command(adbPath, "-s", deviceID, "shell", "getprop", "sys.boot_completed").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
}

func listAVDs(emulatorPath string) ([]string, error) {
	out, err := exec.Command(emulatorPath, "-list-avds").Output()
	if err != nil {
		return nil, err
	}
	var avds []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			avds = append(avds, line)
		}
	}
	return avds, nil
}

func extractPackageName(jsonStr string) string {
	// Simple JSON key extraction without full parser
	marker := `"package_name": "`
	idx := strings.Index(jsonStr, marker)
	if idx < 0 {
		return "com.goleo.app"
	}
	start := idx + len(marker)
	end := strings.Index(jsonStr[start:], `"`)
	if end < 0 {
		return "com.goleo.app"
	}
	return jsonStr[start : start+end]
}
