package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new [project-name]",
	Short: "Create a new Goleo project",
	Args:  cobra.ExactArgs(1),
	RunE:  runNew,
}

type projectConfig struct {
	Name       string
	ModuleName string
}

func runNew(cmd *cobra.Command, args []string) error {
	name := args[0]
	dir := filepath.Join(".", name)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		return fmt.Errorf("directory %s already exists", dir)
	}

	cfg := projectConfig{
		Name:       name,
		ModuleName: fmt.Sprintf("goleo/%s", name),
	}

	fmt.Printf("Creating new Goleo project: %s\n", name)
	fmt.Println()

	for _, sub := range []string{
		filepath.Join("backend", "commands"),
		filepath.Join("backend", "gomobile"),
		filepath.Join("backend", "frontend", "dist"),
		filepath.Join("frontend", "src"),
		filepath.Join("frontend", "dist"),
	} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return fmt.Errorf("failed to create %s dir: %w", sub, err)
		}
	}

	files := map[string]string{
		"backend/main.go":                   tmplMainGo,
		"backend/init.js":                   tmplInitJS,
		"backend/commands/commands.go":      tmplBackendCommandsGo,
		"backend/gomobile/gomobile.go":      tmplMobileGo,
		"backend/gomobile/gomobile_dev.go":  tmplMobileDevGo,
		"backend/gomobile/notifier.go":      tmplMobileNotifierGo,
		"backend/frontend/dist/.gitkeep":    "",
		"go.mod":                            tmplGoMod,
		"frontend/package.json":             tmplFrontendPackageJSON,
		"frontend/index.html":               tmplIndexHTML,
		"frontend/vite.config.ts":           tmplViteConfig,
		"frontend/tsconfig.json":            tmplTsconfig,
		"frontend/env.d.ts":                 tmplEnvDTS,
		"frontend/src/main.ts":              tmplMainTS,
		"frontend/src/App.vue":              tmplAppVue,
		"frontend/src/style.css":            tmplStyleCSS,
		"frontend/dist/.gitkeep":            "",
		"package.json":                      tmplRootPackageJSON,
		"goleo.json":                        tmplGoleoJSON,
	}

	for relPath, content := range files {
		fullPath := filepath.Join(dir, relPath)
		rendered, err := renderTemplate(content, cfg)
		if err != nil {
			return fmt.Errorf("failed to render %s: %w", relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(rendered), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", relPath, err)
		}
		fmt.Printf("  created %s\n", relPath)
	}

	fmt.Println()

	fmt.Println("  Resolving Go dependencies...")
	if err := ensureLocalReplace(dir); err != nil {
		fmt.Printf("  Warning: %v\n", err)
		fmt.Println()
		fmt.Println("  Before running goleo dev, build, or emulate, set GOLEO_ROOT:")
		fmt.Println("    $env:GOLEO_ROOT = \"C:\\path\\to\\goleo\"")
		fmt.Println("  Then run from the project directory:")
		fmt.Println("    go mod edit -replace github.com/daforester/goleo=$env:GOLEO_ROOT")
		fmt.Println("    go mod tidy")
	} else {
		tidy := exec.Command("go", "mod", "tidy")
		tidy.Dir = dir
		tidy.Stdout = os.Stdout
		tidy.Stderr = os.Stderr
		if err := tidy.Run(); err != nil {
			fmt.Printf("  Warning: go mod tidy failed: %v\n", err)
		}
	}

	fmt.Println()
	fmt.Println("Project created successfully!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", name)
	fmt.Println("  cd frontend && npm install && cd ..")
	fmt.Println("  goleo dev          # Start development mode")
	fmt.Println("  goleo build        # Build for current platform")
	fmt.Println()

	linkBridge(dir)

	return nil
}

func linkBridge(projectDir string) {
	goleoRoot := findGoleoRoot()
	if goleoRoot == "" {
		goleoRoot = replaceTargetFromGoMod(projectDir, goleoModule)
	}
	if goleoRoot == "" {
		return
	}

	bridgeDir := filepath.Join(goleoRoot, "bridge")
	if _, err := os.Stat(filepath.Join(bridgeDir, "package.json")); os.IsNotExist(err) {
		return
	}

	fmt.Println("  Linking @goleo/bridge from local source...")
	link := exec.Command("npm", "link")
	link.Dir = bridgeDir
	link.Stdout = os.Stdout
	link.Stderr = os.Stderr
	if err := link.Run(); err != nil {
		fmt.Printf("  Warning: could not link @goleo/bridge: %v\n", err)
		return
	}

	use := exec.Command("npm", "link", "@goleo/bridge")
	use.Dir = filepath.Join(projectDir, "frontend")
	use.Stdout = os.Stdout
	use.Stderr = os.Stderr
	if err := use.Run(); err != nil {
		fmt.Printf("  Warning: could not use linked @goleo/bridge: %v\n", err)
	}
}

func renderTemplate(tmpl string, data projectConfig) (string, error) {
	t, err := template.New("").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}


