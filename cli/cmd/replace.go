package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const goleoModule = "github.com/daforester/goleo"

func ensureLocalReplace(projectDir string) error {
	hasReplace, err := goModHasReplace(projectDir, goleoModule)
	if err != nil {
		return fmt.Errorf("checking go.mod: %w", err)
	}
	if hasReplace {
		return nil
	}

	goleoRoot := findGoleoRoot()
	if goleoRoot == "" {
		return fmt.Errorf("github.com/daforester/goleo is not published yet.\nSet GOLEO_ROOT then re-run:\n  $env:GOLEO_ROOT = \"C:\\path\\to\\goleo\"\n  go mod edit -replace github.com/daforester/goleo=$env:GOLEO_ROOT\n  go mod tidy")
	}

	absRoot, _ := filepath.Abs(goleoRoot)
	replaceDir := filepath.ToSlash(absRoot)
	fmt.Printf("  Adding local replace directive: %s => %s\n", goleoModule, replaceDir)

	replaceCmd := exec.Command("go", "mod", "edit", "-replace", fmt.Sprintf("%s=%s", goleoModule, replaceDir))
	replaceCmd.Dir = projectDir
	replaceCmd.Stdout = os.Stdout
	replaceCmd.Stderr = os.Stderr
	if err := replaceCmd.Run(); err != nil {
		return fmt.Errorf("failed to add replace directive: %w", err)
	}

	return nil
}

func findGoleoRoot() string {
	if root := os.Getenv("GOLEO_ROOT"); root != "" {
		if _, err := os.Stat(filepath.Join(root, "runtime", "app.go")); err == nil {
			return root
		}
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		// for npm installation: bin/goleo.exe, goleo/ at package root
		if _, err := os.Stat(filepath.Join(exeDir, "..", "goleo", "runtime", "app.go")); err == nil {
			return filepath.Join(exeDir, "..", "goleo")
		}
		for i := 0; i < 5; i++ {
			if _, err := os.Stat(filepath.Join(exeDir, "runtime", "app.go")); err == nil {
				return exeDir
			}
			if _, err := os.Stat(filepath.Join(exeDir, "go.mod")); err == nil {
				if data, err := os.ReadFile(filepath.Join(exeDir, "go.mod")); err == nil {
					if strings.Contains(string(data), "module github.com/daforester/goleo") {
						return exeDir
					}
				}
			}
			parent := filepath.Dir(exeDir)
			if parent == exeDir {
				break
			}
			exeDir = parent
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(cwd, "runtime", "app.go")); err == nil {
			return cwd
		}
	}

	return ""
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

func replaceTargetFromGoMod(projectDir, module string) string {
	data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "replace ") && strings.Contains(trimmed, module+" =>") {
			parts := strings.SplitN(trimmed, "=>", 2)
			if len(parts) == 2 {
				target := strings.TrimSpace(parts[1])
				if _, err := os.Stat(filepath.Join(target, "bridge", "package.json")); err == nil {
					return target
				}
			}
		}
	}
	return ""
}
