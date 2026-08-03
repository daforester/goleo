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

	// Already vendored and already consistent — leave it alone. Without this,
	// ensureGoleoRequire's `go get` runs unconditionally on every single
	// dev/build/emulate invocation, so a lagging module proxy or checksum
	// database (routinely the case for a few minutes right after a fresh
	// goleo release — see AGENTS.md/SPIKES.md) can re-pin go.mod to a
	// different version than what's already vendored, corrupting an
	// otherwise-healthy project with an "inconsistent vendoring" error on the
	// very next `go build`/`go run`. Only fall through to network resolution
	// when go.mod has no require yet (fresh scaffold) or it actively
	// disagrees with vendor/modules.txt (needs reconciling).
	if _, err := os.Stat(filepath.Join(projectDir, "vendor", "modules.txt")); err == nil {
		if reqV := requiredGoleoVersion(projectDir); reqV != "" && reqV == vendoredGoleoVersion(projectDir) {
			return nil
		}
	}

	// End users: resolve from the module proxy.
	return ensureGoleoRequire(projectDir)
}

// requiredGoleoVersion returns the version (without the "v" prefix) go.mod
// currently requires for goleoModule, or "" if there's no require for it yet.
func requiredGoleoVersion(projectDir string) string {
	data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`(?:^|\s)` + regexp.QuoteMeta(goleoModule) + `\s+v(\S+)`)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "=>") {
			continue // a replace directive, not a require
		}
		if m := re.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			return m[1]
		}
	}
	return ""
}

// vendoredGoleoVersion returns the version vendor/modules.txt records as the
// explicit (directly required) entry for goleoModule, or "" if the project
// isn't vendored or doesn't vendor it explicitly.
func vendoredGoleoVersion(projectDir string) string {
	data, err := os.ReadFile(filepath.Join(projectDir, "vendor", "modules.txt"))
	if err != nil {
		return ""
	}
	prefix := "# " + goleoModule + " v"
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "## explicit") {
			return strings.TrimPrefix(trimmed, prefix)
		}
	}
	return ""
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
	// Drop the current require before falling back. `go get X@latest` still has to
	// load the module graph first, so an UNRESOLVABLE pin already in go.mod makes
	// even this recovery path fail:
	//
	//   go: github.com/daforester/goleo@v0.8.8: reading .../go.mod at revision
	//   v0.8.8: unknown revision v0.8.8
	//
	// which is precisely the situation the fallback exists for — a scaffold pinned
	// to this CLI's version (see scaffoldGoleoVersion) during the window after an
	// npm publish but before the matching Go tag is fetchable, or any pin that was
	// later yanked. Dropping it first lets @latest resolve cleanly; it's a no-op
	// when the module isn't required.
	if err := runGo(projectDir, modModEnv(), "mod", "edit", "-droprequire", goleoModule); err != nil {
		return fmt.Errorf("clearing the unresolved %s require before falling back: %w", goleoModule, err)
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

// scaffoldPlaceholderVersion is what a scaffolded go.mod requires when the CLI
// can't name its own published version (an unstamped `go build` of the CLI, where
// resolveVersion() == "dev"). It is deliberately a version that does NOT exist on
// the proxy, and it is the same idiom the spike modules use alongside a local
// replace. That combination is what makes it the right placeholder:
//   - developing goleo itself (GOLEO_ROOT / a local replace) needs *some* require
//     for the replace to apply, and v0.0.0 is it;
//   - if it is ever left unresolved instead, `go build` fails loudly on an
//     unknown revision rather than silently compiling against a real-but-ancient
//     API — which is exactly what the previously hardcoded v0.2.1 did.
const scaffoldPlaceholderVersion = "v0.0.0"

// scaffoldGoleoVersion returns the goleo version a freshly scaffolded project
// should require, as a `v`-prefixed string.
//
// It pins the CLI's OWN version, so a new project starts consistent with the
// binary that created it and reproducible without a network round-trip (the
// module is usually already in the local cache). This is also self-maintaining:
// `npm version` stamps the CLI at release time, so the scaffold tracks releases
// automatically instead of needing a hardcoded string bumped by hand — the old
// hardcoded `v0.2.1` had gone stale by dozens of releases.
//
// ensureGoleoRequire still runs afterwards and will move this forward (or fall
// back to @latest) if the exact version isn't on the proxy yet; this only decides
// where the project *starts*.
func scaffoldGoleoVersion() string {
	if v := resolveVersion(); releaseVersionRe.MatchString(v) {
		return "v" + v
	}
	return scaffoldPlaceholderVersion
}

// releaseVersionRe matches only an exact published release (`1.2.3`), which is
// deliberately stricter than semverRe's prefix match.
//
// resolveVersion() falls back to debug.ReadBuildInfo(), and for a locally-built
// CLI that yields a PSEUDO-version of an unpushed commit with build metadata —
// e.g. `0.8.8-0.20260803161633-d8da50055a4e+dirty`. semverRe's prefix match
// accepts that (it starts `0.8.8`), which would bake a reference to a commit
// nobody can fetch into every scaffolded go.mod. Only a clean tag is safe to pin:
// an ldflags-stamped release build, or `go install ...@v1.2.3` (whose build info
// reports that tag). Anything else falls back to scaffoldPlaceholderVersion.
var releaseVersionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

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
