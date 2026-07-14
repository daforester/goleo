package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	installAPK    string
	installLaunch bool
)

var installCmd = &cobra.Command{
	Use:   "install [target]",
	Short: "Install (sideload) the built app onto a connected device",
	Long: `Install a built Goleo app onto a connected real device (or a running emulator).

Currently supports Android: installs an already-built APK via adb onto the
connected device, then launches it. Build the APK first with 'goleo build android'
(which writes app.apk), or use the 'goleo:sideload-android' npm script to do both.

Targets:
  android    adb install the APK onto the connected device / running emulator`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInstall,
}

func init() {
	installCmd.Flags().StringVar(&installAPK, "apk", "app.apk", "APK to install (android)")
	installCmd.Flags().BoolVar(&installLaunch, "launch", true, "Launch the app after installing (android)")
}

func runInstall(cmd *cobra.Command, args []string) error {
	target := "android"
	if len(args) > 0 {
		target = strings.ToLower(args[0])
	}
	switch target {
	case "android":
		return installAndroid()
	default:
		return fmt.Errorf("unsupported target: %s (supported: android)", target)
	}
}

func installAndroid() error {
	if _, err := os.Stat(installAPK); err != nil {
		return fmt.Errorf("%s not found — run `goleo build android` first (or pass --apk)", installAPK)
	}
	deps, err := ensureAndroidDeps()
	if err != nil {
		return err
	}
	if deps.AdbPath == "" {
		return fmt.Errorf("adb not found: install Android platform-tools")
	}
	// Require a connected device / running emulator; do NOT auto-start one — the
	// intent is sideloading to hardware that's already attached.
	deviceID := findRunningDevice(deps.AdbPath)
	if deviceID == "" {
		return fmt.Errorf("no device connected. Plug in a device with USB debugging enabled (or start an emulator), then re-run")
	}

	apkPath, _ := filepath.Abs(installAPK)
	fmt.Printf("  Installing %s on %s...\n", installAPK, deviceID)
	install := exec.Command(deps.AdbPath, "-s", deviceID, "install", "-r", "-d", apkPath) // #nosec G204 -- adb path + local apk
	install.Stdout = os.Stdout
	install.Stderr = os.Stderr
	if err := install.Run(); err != nil {
		return fmt.Errorf("adb install failed: %w", err)
	}

	if installLaunch {
		pkgName := "com.goleo.app"
		if data, err := os.ReadFile("goleo.json"); err == nil {
			pkgName = extractPackageName(string(data))
		}
		launch := exec.Command(deps.AdbPath, "-s", deviceID, "shell", "am", "start", "-n", pkgName+"/.MainActivity") // #nosec G204
		launch.Stdout = os.Stdout
		launch.Stderr = os.Stderr
		_ = launch.Run() // best-effort: install succeeded even if launch fails
	}
	fmt.Println("  Installed.")
	return nil
}
