package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// goBinDir returns the directory `go install` places compiled binaries in:
// $GOBIN when set, otherwise the first $GOPATH entry plus /bin, falling back to
// ~/go/bin. Tools such as gomobile land here, and this directory is frequently
// missing from the user's PATH — which is why locating the binary directly and
// exposing this dir to child processes is more robust than relying on PATH.
func goBinDir() string {
	if out, err := exec.Command("go", "env", "GOBIN", "GOPATH").Output(); err == nil {
		lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
		if len(lines) >= 1 {
			if gobin := strings.TrimSpace(lines[0]); gobin != "" {
				return gobin
			}
		}
		if len(lines) >= 2 {
			if gopath := strings.TrimSpace(lines[1]); gopath != "" {
				// GOPATH may be a list; `go install` writes to the first entry.
				return filepath.Join(filepath.SplitList(gopath)[0], "bin")
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "go", "bin")
	}
	return ""
}

// exeName appends the platform executable suffix (.exe on Windows).
func exeName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// findTool locates a go-installed command by name: first on PATH, then in the
// Go bin directory where `go install` would have placed it.
func findTool(name string) (string, bool) {
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	if dir := goBinDir(); dir != "" {
		candidate := filepath.Join(dir, exeName(name))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// prependPath returns env with dir prepended to its PATH entry (matched
// case-insensitively so it also handles Windows' "Path"), adding one if absent.
func prependPath(env []string, dir string) []string {
	if dir == "" {
		return env
	}
	for i, e := range env {
		if eq := strings.IndexByte(e, '='); eq >= 0 && strings.EqualFold(e[:eq], "PATH") {
			env[i] = e[:eq+1] + dir + string(os.PathListSeparator) + e[eq+1:]
			return env
		}
	}
	return append(env, "PATH="+dir)
}

// goToolEnv returns the current environment with the Go bin directory prepended
// to PATH, so a go-installed tool (e.g. gomobile) can find the other tools it
// shells out to (e.g. gobind) even when GOPATH/bin is not on the user's PATH.
func goToolEnv() []string {
	return prependPath(os.Environ(), goBinDir())
}
