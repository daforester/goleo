package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const goleoModule = "github.com/daforester/goleo"

// ensureGoleoResolvable makes sure a scaffolded project can build against
// github.com/daforester/goleo.
//
// For end users this is just a normal published Go dependency (like `tauri` on
// crates.io): the project's go.mod requires a published version and `go mod tidy`
// fetches it from the module proxy — no local `replace`, no bundled source, no
// absolute paths, no env vars. We only run `go get` to make sure the require
// points at a version that actually exists (older scaffolds pinned an
// unpublished placeholder).
//
// The single exception is developing goleo itself: set GOLEO_ROOT to a local
// checkout and it's wired in via a `replace`.
//
// (Named ensureLocalReplace for historical call-site compatibility.)
func ensureLocalReplace(projectDir string) error {
	// Already pinned to a local checkout (a goleo dev) — leave it.
	hasReplace, err := goModHasReplace(projectDir, goleoModule)
	if err != nil {
		return fmt.Errorf("checking go.mod: %w", err)
	}
	if hasReplace {
		return nil
	}

	// Developing goleo itself: GOLEO_ROOT => local checkout via a replace.
	if root := os.Getenv("GOLEO_ROOT"); root != "" {
		if _, statErr := os.Stat(filepath.Join(root, "runtime", "app.go")); statErr == nil {
			absRoot, _ := filepath.Abs(root)
			target := filepath.ToSlash(absRoot)
			fmt.Printf("  Using local goleo checkout (GOLEO_ROOT): %s => %s\n", goleoModule, target)
			return runGo(projectDir, nil, "mod", "edit", "-replace", goleoModule+"="+target)
		}
		return fmt.Errorf("GOLEO_ROOT=%q does not contain runtime/app.go", root)
	}

	// End users: resolve from the module proxy.
	return ensureGoleoRequire(projectDir)
}

// ensureGoleoRequire points the project's go.mod at a published goleo version and
// lets `go mod tidy` (run by the caller) fetch it from the proxy. It pins to the
// CLI's own version for reproducibility, falling back to @latest if that exact
// version isn't tagged as a Go module yet.
func ensureGoleoRequire(projectDir string) error {
	spec := "latest"
	if v := resolveVersion(); semverRe.MatchString(v) {
		spec = "v" + v
	}
	// `go get` refuses to run under -mod=vendor (scaffolds commit a vendor/), so
	// force -mod=mod to resolve from the module cache/proxy.
	if err := runGo(projectDir, modModEnv(), "get", goleoModule+"@"+spec); err == nil {
		return nil
	} else if spec != "latest" {
		fmt.Printf("  %s@%s not available on the proxy yet — using @latest\n", goleoModule, spec)
		if err2 := runGo(projectDir, modModEnv(), "get", goleoModule+"@latest"); err2 == nil {
			return nil
		}
		return err
	} else {
		return fmt.Errorf("could not resolve %s from the Go module proxy: %w\n"+
			"Check your network connection (the first build needs to download it),\n"+
			"or, if developing goleo itself, set GOLEO_ROOT to your local checkout.", goleoModule, err)
	}
}

var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+`)

func runGo(dir string, env []string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func parseModuleName(projectDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "module ")), nil
		}
	}
	return "", fmt.Errorf("module directive not found in go.mod")
}

func goModHasReplace(projectDir, module string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("go.mod not found")
		}
		return false, err
	}
	return containsReplace(string(data), module), nil
}

func containsReplace(modContent, module string) bool {
	lines := strings.Split(modContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "replace ") && strings.Contains(trimmed, module+" =>") {
			return true
		}
	}
	return false
}
