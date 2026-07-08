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
	"strings"
)

type androidDeps struct {
	JavaHome      string
	SDKRoot       string
	NDKDir        string
	Gomobile      string
	AdbPath       string
	EmulatorPath  string
}

const (
	goleoAndroidDir = ".goleo/android"
)

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

func (d *androidDeps) resolveJava() error {
	javaHome := os.Getenv("JAVA_HOME")
	if javaHome != "" {
		javac := filepath.Join(javaHome, "bin", "javac")
		if runtime.GOOS == "windows" {
			javac += ".exe"
		}
		if _, err := os.Stat(javac); err == nil {
			d.JavaHome = javaHome
			ver := exec.Command(filepath.Join(javaHome, "bin", "java"), "-version")
			ver.Stderr = os.Stderr
			return nil
		}
	}

	if path, err := exec.LookPath("javac"); err == nil {
		parent := filepath.Dir(filepath.Dir(path))
		if _, err := os.Stat(filepath.Join(parent, "bin", "javac")); err == nil {
			d.JavaHome = parent
			return nil
		}
	}

	if path, err := exec.LookPath("java"); err == nil {
		// Try to find JAVA_HOME from java location
		javaBin := filepath.Dir(path)
		parent := filepath.Dir(javaBin)
		if _, err := os.Stat(filepath.Join(parent, "lib", "tools.jar")); err == nil {
			d.JavaHome = parent
			return nil
		}
		if _, err := os.Stat(filepath.Join(parent, "jre")); err == nil {
			d.JavaHome = parent
			return nil
		}
		if _, err := os.Stat(filepath.Join(parent, "bin", "javac")); err == nil {
			d.JavaHome = parent
			return nil
		}
		// Check common JDK subdirectories
		for _, name := range []string{"jdk", "jdk-17", "jdk-21", "jdk-11"} {
			candidate := filepath.Join(parent, name)
			if _, err := os.Stat(filepath.Join(candidate, "bin", "javac")); err == nil {
				d.JavaHome = candidate
				return nil
			}
		}
		// Fall through to common paths
	}

	// Check common install paths
	common := commonJavaPaths()
	for _, p := range common {
		if _, err := os.Stat(filepath.Join(p, "bin", "javac")); err == nil {
			d.JavaHome = p
			return nil
		}
	}

	return d.installJava()
}

func (d *androidDeps) installJava() error {
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
	installDir := filepath.Join(goleoAndroidDir, "jdk")
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
	if path, err := exec.LookPath("gomobile"); err == nil {
		d.Gomobile = path
		return nil
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

	if path, err := exec.LookPath("gomobile"); err == nil {
		d.Gomobile = path
		return nil
	}

	return fmt.Errorf("gomobile installed but not found in PATH. Ensure GOPATH/bin is in your PATH")
}

func (d *androidDeps) resolveSDK() error {
	sdkRoot := os.Getenv("ANDROID_HOME")
	if sdkRoot == "" {
		sdkRoot = os.Getenv("ANDROID_SDK_ROOT")
	}

	if sdkRoot != "" {
		if _, err := os.Stat(filepath.Join(sdkRoot, "platforms")); err == nil {
			d.SDKRoot = sdkRoot
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
			d.SDKRoot = p
			return nil
		}
	}

	return d.installSDK()
}

func (d *androidDeps) installSDK() error {
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

	installDir := filepath.Join(goleoAndroidDir, "sdk")
	os.MkdirAll(installDir, 0755)

	fmt.Println("  Downloading Android command-line tools...")
	url := sdkManagerURL()
	archivePath := filepath.Join(installDir, "cmdline-tools.zip")

	if err := downloadFile(url, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	fmt.Println("  Extracting...")
	extractDir := filepath.Join(installDir, "cmdline-tools")
	os.MkdirAll(extractDir, 0755)
	if _, err := unzipAndFind(archivePath, extractDir, nil); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Find sdkmanager
	sdkmanager := findFile(extractDir, "sdkmanager")
	if sdkmanager == "" {
		// Try nested structure: cmdline-tools/latest/bin/sdkmanager
		nestedDir := filepath.Join(extractDir, "cmdline-tools")
		if entries, _ := os.ReadDir(extractDir); len(entries) == 1 {
			nestedDir = filepath.Join(extractDir, entries[0].Name())
		}
		os.MkdirAll(filepath.Join(installDir, "cmdline-tools", "latest"), 0755)
		if err := moveDir(nestedDir, filepath.Join(installDir, "cmdline-tools", "latest")); err == nil {
			sdkmanager = filepath.Join(installDir, "cmdline-tools", "latest", "bin", "sdkmanager")
			if runtime.GOOS == "windows" {
				sdkmanager += ".bat"
			}
		}
	}

	if sdkmanager == "" || os.Getenv("CI") != "" {
		fmt.Println("  Downloading essential SDK components directly...")
	} else {
		fmt.Println("  Installing essential SDK components via sdkmanager...")
		yes := exec.Command("sh", "-c", fmt.Sprintf("yes | %s \"platform-tools\" \"platforms;android-34\" \"build-tools;34.0.0\"", sdkmanager))
		yes.Dir = installDir
		yes.Stdout = os.Stdout
		yes.Stderr = os.Stderr
		yes.Run()
	}

	d.SDKRoot = installDir
	fmt.Printf("  Android SDK installed at: %s\n", installDir)
	os.Setenv("ANDROID_HOME", installDir)
	return nil
}

func (d *androidDeps) resolveNDK() error {
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

	sdkmanager := findFile(d.SDKRoot, "sdkmanager")
	if sdkmanager == "" {
		fmt.Println("  sdkmanager not found. Cannot auto-install NDK.")
		fmt.Println("  Install Android Studio or download NDK manually:")
		fmt.Println("    https://developer.android.com/ndk/downloads")
		return fmt.Errorf("Android NDK is required")
	}

	fmt.Println("  Installing NDK via sdkmanager...")
	install := exec.Command("sh", "-c", fmt.Sprintf("yes | %s \"ndk;25.2.9519653\"", sdkmanager))
	install.Dir = d.SDKRoot
	install.Stdout = os.Stdout
	install.Stderr = os.Stderr
	if err := install.Run(); err != nil {
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
			sdkmanager := findFile(d.SDKRoot, "sdkmanager")
			if sdkmanager != "" {
				install := exec.Command("sh", "-c", fmt.Sprintf("yes | %s \"platform-tools\"", sdkmanager))
				install.Dir = d.SDKRoot
				install.Stdout = os.Stdout
				install.Stderr = os.Stderr
				if err := install.Run(); err == nil {
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
	return nil
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

		dst, err := os.Create(target)
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
