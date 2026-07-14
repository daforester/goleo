package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ensureMakensis returns a path to the NSIS `makensis` compiler, auto-installing
// NSIS via the host's package manager when it isn't already present. This is what
// lets `goleo build windows --bundle` "just work" on a fresh machine. Set
// GOLEO_NO_INSTALL=1 to disable the auto-install and get a plain error instead.
func ensureMakensis() (string, error) {
	if p := findMakensis(); p != "" {
		return p, nil
	}
	if os.Getenv("GOLEO_NO_INSTALL") == "1" {
		return "", fmt.Errorf("bundle: makensis not found — install NSIS (https://nsis.sourceforge.io) and retry")
	}

	fmt.Println("  makensis (NSIS) not found — installing it...")
	if err := installNSIS(); err != nil {
		return "", fmt.Errorf("bundle: makensis not found and auto-install failed: %w\n"+
			"  Install NSIS manually from https://nsis.sourceforge.io and retry (or set GOLEO_NO_INSTALL=1)", err)
	}

	if p := findMakensis(); p != "" {
		fmt.Println("  NSIS installed:", p)
		return p, nil
	}
	return "", fmt.Errorf("bundle: NSIS was installed but makensis could not be located — " +
		"reopen your terminal so PATH refreshes, then retry")
}

// findMakensis locates makensis on PATH, the Go bin dir, or (on Windows) the
// standard NSIS install directory, which the installer does not add to PATH.
func findMakensis() string {
	if p, ok := findTool("makensis"); ok {
		return p
	}
	if runtime.GOOS == "windows" {
		bases := []string{
			os.Getenv("ProgramFiles(x86)"),
			os.Getenv("ProgramFiles"),
			`C:\Program Files (x86)`,
			`C:\Program Files`,
		}
		for _, base := range bases {
			if base == "" {
				continue
			}
			cand := filepath.Join(base, "NSIS", "makensis.exe")
			if info, err := os.Stat(cand); err == nil && !info.IsDir() {
				return cand
			}
		}
	}
	return ""
}

// installNSIS installs NSIS using whichever package manager the host provides.
func installNSIS() error {
	switch runtime.GOOS {
	case "windows":
		if _, err := exec.LookPath("winget"); err == nil {
			return runPackageInstall("winget", "install", "-e", "--id", "NSIS.NSIS",
				"--accept-source-agreements", "--accept-package-agreements")
		}
		if _, err := exec.LookPath("choco"); err == nil {
			return runPackageInstall("choco", "install", "nsis", "-y")
		}
		if _, err := exec.LookPath("scoop"); err == nil {
			return runPackageInstall("scoop", "install", "nsis")
		}
		return fmt.Errorf("no package manager found (winget, choco, or scoop)")
	case "darwin":
		if _, err := exec.LookPath("brew"); err == nil {
			return runPackageInstall("brew", "install", "nsis")
		}
		return fmt.Errorf("Homebrew not found (needed for: brew install nsis)")
	case "linux":
		if _, err := exec.LookPath("apt-get"); err == nil {
			return runPackageInstall("sudo", "apt-get", "install", "-y", "nsis")
		}
		if _, err := exec.LookPath("dnf"); err == nil {
			return runPackageInstall("sudo", "dnf", "install", "-y", "mingw32-nsis")
		}
		if _, err := exec.LookPath("pacman"); err == nil {
			return runPackageInstall("sudo", "pacman", "-S", "--noconfirm", "nsis")
		}
		return fmt.Errorf("no supported package manager found (apt-get, dnf, or pacman)")
	}
	return fmt.Errorf("auto-install not supported on %s", runtime.GOOS)
}

func runPackageInstall(name string, args ...string) error {
	fmt.Printf("  $ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...) // #nosec G204 -- fixed package-manager invocation
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
