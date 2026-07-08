//go:build linux

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// webkit4xRe matches WebKitGTK pkg-config modules in the 4.x line
// (webkit2gtk-4.0, -4.1, -4.2, ...). The bundled webview library
// (github.com/webview/webview_go) hardcodes a cgo dependency on webkit2gtk-4.0
// and targets that line's C API (<webkit2/webkit2.h>); releases within it stay
// API-compatible for the symbols webview uses, so any 4.x — including ones not
// yet released — is accepted by discovering them at runtime rather than
// matching a fixed list.
//
// webkitgtk-6.0 is deliberately NOT matched: it is a different, GTK4-based API
// that this webview version cannot compile against, so silently shimming to it
// would only trade one build failure for a more confusing one.
var webkit4xRe = regexp.MustCompile(`^webkit2gtk-4\.(\d+)$`)

// prepareWebkitEnv inspects pkg-config and returns environment overrides (to be
// appended to a command's Env) that let cgo find an installed WebKitGTK. It
// returns nil when webkit2gtk-4.0 is present, or an error with install guidance
// when no compatible WebKitGTK is available.
func prepareWebkitEnv() ([]string, error) {
	if _, err := exec.LookPath("pkg-config"); err != nil {
		return nil, fmt.Errorf("pkg-config not found on PATH; install pkg-config and a WebKitGTK dev package (e.g. webkit2gtk-4.1) to build the desktop window")
	}

	// Discover every webkit2gtk-4.x module the system actually advertises,
	// sorted oldest-to-newest. This is what makes the check future-proof: a
	// distro that ships only a newer 4.x (say webkit2gtk-4.2) is handled with
	// no code change.
	out, _ := exec.Command("pkg-config", "--list-all").Output()
	installed, needShim, ok := chooseWebkit(parseWebkit4x(string(out)))
	if !ok {
		return nil, fmt.Errorf("no compatible WebKitGTK (webkit2gtk-4.x) found via pkg-config.\n" +
			"  Install a WebKitGTK development package, for example:\n" +
			"    Debian/Ubuntu:  sudo apt install libwebkit2gtk-4.1-dev\n" +
			"    Fedora:         sudo dnf install webkit2gtk4.1-devel\n" +
			"    Arch:           sudo pacman -S webkit2gtk-4.1")
	}
	if !needShim {
		// webkit2gtk-4.0 is present, which is exactly what the webview library
		// asks pkg-config for; nothing to do.
		return nil, nil
	}

	shimDir, err := writeWebkitShim(installed)
	if err != nil {
		return nil, fmt.Errorf("setting up WebKitGTK compatibility shim: %w", err)
	}

	fmt.Printf("  WebKitGTK: webkit2gtk-4.0 not installed; linking against %s (%s)\n", installed, pkgConfigVersion(installed))

	pcPath := shimDir
	if existing := os.Getenv("PKG_CONFIG_PATH"); existing != "" {
		pcPath = shimDir + string(os.PathListSeparator) + existing
	}
	return []string{"PKG_CONFIG_PATH=" + pcPath}, nil
}

// parseWebkit4x extracts the webkit2gtk-4.x module names from `pkg-config
// --list-all` output, sorted by minor version ascending (e.g.
// ["webkit2gtk-4.0", "webkit2gtk-4.1"]). It is separated from the exec call so
// the parsing and version selection can be unit-tested with canned input.
func parseWebkit4x(listAllOutput string) []string {
	var minors []int
	seen := map[int]bool{}
	for _, line := range strings.Split(listAllOutput, "\n") {
		// Each line is "<module-name><whitespace><description>".
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if m := webkit4xRe.FindStringSubmatch(fields[0]); m != nil {
			if minor, err := strconv.Atoi(m[1]); err == nil && !seen[minor] {
				seen[minor] = true
				minors = append(minors, minor)
			}
		}
	}

	sort.Ints(minors)
	mods := make([]string, len(minors))
	for i, minor := range minors {
		mods[i] = fmt.Sprintf("webkit2gtk-4.%d", minor)
	}
	return mods
}

// chooseWebkit decides, given the sorted-ascending list of installed
// webkit2gtk-4.x modules, which module cgo should link against.
//
//	ok == false          -> none installed; caller reports an install hint
//	needShim == false    -> webkit2gtk-4.0 is present; use it natively
//	needShim == true     -> 4.0 absent; shim toward `target` (newest 4.x)
func chooseWebkit(mods []string) (target string, needShim, ok bool) {
	if len(mods) == 0 {
		return "", false, false
	}
	for _, name := range mods {
		if name == "webkit2gtk-4.0" {
			return name, false, true
		}
	}
	return mods[len(mods)-1], true, true
}

func pkgConfigVersion(name string) string {
	out, err := exec.Command("pkg-config", "--modversion", name).Output()
	if err != nil {
		return "unknown version"
	}
	return strings.TrimSpace(string(out))
}

// writeWebkitShim creates a directory containing a webkit2gtk-4.0.pc file that
// Requires the installed WebKitGTK module, and returns the directory so it can
// be prepended to PKG_CONFIG_PATH. pkg-config merges the Requires target's
// cflags/libs, giving cgo the correct include path (e.g. webkitgtk-4.1) where
// <webkit2/webkit2.h> lives.
func writeWebkitShim(installed string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	shimDir := filepath.Join(cacheDir, "goleo", "pkgconfig")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		return "", err
	}

	pc := fmt.Sprintf(`# Generated by goleo: redirects webkit2gtk-4.0 -> %s
Name: webkit2gtk-4.0
Description: goleo compatibility shim redirecting webkit2gtk-4.0 to %s
Version: %s
Requires: %s
`, installed, installed, pkgConfigVersion(installed), installed)

	pcPath := filepath.Join(shimDir, "webkit2gtk-4.0.pc")
	if err := os.WriteFile(pcPath, []byte(pc), 0o644); err != nil {
		return "", err
	}
	return shimDir, nil
}
