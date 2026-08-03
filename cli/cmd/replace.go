package cmd

import (
	"bytes"
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
	// Try the CLI's exact version first (reproducible), but quietly — the
	// matching Go-module git tag may not exist yet if npm was published without
	// it, and `go get`'s raw 404 output would look alarming. Only if that misses
	// do we fall back to @latest, visibly.
	if v := resolveVersion(); semverRe.MatchString(v) {
		if _, err := goGetQuiet(projectDir, goleoModule+"@v"+v); err == nil {
			return nil
		}
		fmt.Printf("  %s@v%s not tagged as a Go module yet — using @latest\n", goleoModule, v)
	}
	// `go get` refuses to run under -mod=vendor (scaffolds commit a vendor/), so
	// force -mod=mod to resolve from the module cache/proxy.
	if err := runGo(projectDir, modModEnv(), "get", goleoModule+"@latest"); err != nil {
		return fmt.Errorf("could not resolve %s from the Go module proxy: %w\n"+
			"Check your network connection (the first build needs to download it),\n"+
			"or, if developing goleo itself, set GOLEO_ROOT to your local checkout.", goleoModule, err)
	}
	return nil
}

// keepVendorInSync re-runs `go mod vendor` after go.mod may have changed.
// ensureGoleoRequire re-pins the goleo require to the running CLI's own
// version on every dev/build/emulate invocation, not just at `goleo new` —
// so if that exact version wasn't published to the module proxy yet when the
// project was scaffolded (falling back to @latest) but is by the time a
// later command runs, the require bumps forward while the committed
// vendor/modules.txt is left recording the old version. A vendored project
// hard-fails `go build`/`go run` with "inconsistent vendoring" the moment
// go.mod and vendor/modules.txt disagree, so any command that can mutate
// go.mod after creation must re-vendor to match. No-op if the project isn't
// vendored (no vendor/modules.txt); best-effort otherwise, matching `goleo
// new`'s own vendoring step, so a network hiccup here doesn't block the
// build — Go will still surface the real error if vendor/ ends up stale.
func keepVendorInSync(dir string) {
	if _, err := os.Stat(filepath.Join(dir, "vendor", "modules.txt")); err != nil {
		return
	}
	vendor := exec.Command("go", "mod", "vendor")
	vendor.Dir = dir
	vendor.Stdout = os.Stdout
	vendor.Stderr = os.Stderr
	if err := vendor.Run(); err != nil {
		fmt.Printf("  Warning: go mod vendor failed (vendor/ may be stale): %v\n", err)
	}
}

// goGetQuiet runs `go get <spec>` capturing output, so an expected miss (the
// pinned version not being tagged yet) doesn't spew go's raw error.
func goGetQuiet(projectDir, spec string) (string, error) {
	cmd := exec.Command("go", "get", spec)
	cmd.Dir = projectDir
	cmd.Env = modModEnv()
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return buf.String(), err
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
