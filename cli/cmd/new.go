package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new [project-name]",
	Short: "Create a new Goleo project",
	Args:  cobra.ExactArgs(1),
	RunE:  runNew,
}

var (
	newTemplate    string
	newDemo        bool
	newSkipInstall bool
)

func init() {
	newCmd.Flags().StringVar(&newTemplate, "template", "", "Project template: minimal or demo (prompts if omitted and interactive)")
	newCmd.Flags().BoolVar(&newDemo, "demo", false, "Shorthand for --template demo (full host-feature showcase)")
	newCmd.Flags().BoolVar(&newSkipInstall, "no-install", false, "Skip running npm install in the frontend")
}

// chooseTemplate resolves which starter to scaffold: the --template/--demo flags
// win; otherwise it prompts on an interactive terminal, and defaults to
// "minimal" when non-interactive (CI, piped input).
func chooseTemplate() (string, error) {
	t := strings.ToLower(strings.TrimSpace(newTemplate))
	if t == "" && newDemo {
		t = "demo"
	}
	switch t {
	case "minimal", "demo":
		return t, nil
	case "":
		// prompt / default below
	default:
		return "", fmt.Errorf("unknown template %q (want: minimal or demo)", newTemplate)
	}
	if fi, err := os.Stdin.Stat(); err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return "minimal", nil // non-interactive
	}
	fmt.Println("Choose a template:")
	fmt.Println("  1) minimal — a clean starter (default)")
	fmt.Println("  2) demo    — full showcase of every host feature")
	fmt.Print("Template [1/2]: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if s := strings.TrimSpace(line); s == "2" || strings.EqualFold(s, "demo") {
		return "demo", nil
	}
	return "minimal", nil
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

	template, err := chooseTemplate()
	if err != nil {
		return err
	}

	fmt.Printf("Creating new Goleo project: %s (%s template)\n", name, template)
	fmt.Println()

	for _, sub := range []string{
		filepath.Join("backend", "app"),
		filepath.Join("backend", "commands"),
		filepath.Join("backend", "gomobile"),
		filepath.Join("backend", "frontend", "dist"),
		filepath.Join("frontend", "src"),
		filepath.Join("frontend", "public"),
		filepath.Join("frontend", "dist"),
	} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return fmt.Errorf("failed to create %s dir: %w", sub, err)
		}
	}

	if template == "demo" {
		if err := extractDemoTemplate(dir, name); err != nil {
			return fmt.Errorf("extracting demo template: %w", err)
		}
		fmt.Println("  created full-feature demo project")
	} else {
		files := map[string]string{
			"backend/app/app.go":             tmplAppGo,
			"backend/init.js":                tmplInitJS,
			"backend/commands/commands.go":   tmplBackendCommandsGo,
			"backend/frontend/dist/.gitkeep": "",
			"go.mod":                         tmplGoMod,
			"frontend/package.json":          tmplFrontendPackageJSON,
			"frontend/index.html":            tmplIndexHTML,
			"frontend/vite.config.ts":        tmplViteConfig,
			"frontend/tsconfig.json":         tmplTsconfig,
			"frontend/env.d.ts":              tmplEnvDTS,
			"frontend/src/main.ts":           tmplMainTS,
			"frontend/src/App.vue":           tmplAppVue,
			"frontend/src/style.css":         tmplStyleCSS,
			"frontend/public/sw.js":          tmplSWJS,
			"frontend/public/manifest.json":  tmplManifestJSON,
			"frontend/dist/.gitkeep":         "",
			"package.json":                   tmplRootPackageJSON,
			"goleo.json":                     tmplGoleoJSON,
			".gitignore":                     tmplGitignore,
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
	}

	fmt.Println()

	fmt.Println("  Resolving Go dependencies...")
	replaceErr := ensureLocalReplace(dir)
	if replaceErr != nil {
		fmt.Printf("  Note: %v\n", replaceErr)
		fmt.Println("  The project was still created. Go dependencies will resolve on your")
		fmt.Println("  next `goleo dev` / `goleo build`. (A just-published release can take a")
		fmt.Println("  few minutes to propagate to the Go module proxy / checksum DB.)")
	} else {
		tidy := exec.Command("go", "mod", "tidy")
		tidy.Dir = dir
		tidy.Stdout = os.Stdout
		tidy.Stderr = os.Stderr
		if err := tidy.Run(); err != nil {
			fmt.Printf("  Warning: go mod tidy failed: %v\n", err)
		}
	}

	if err := generateBackendEntrypoints(dir); err != nil {
		fmt.Printf("  Warning: could not generate backend entry points: %v\n", err)
		fmt.Println("  They will be generated on the next goleo dev/build/emulate run.")
	} else {
		fmt.Println("  created backend/main.go (generated)")
		fmt.Println("  created backend/gomobile/gomobile.go (generated)")
		fmt.Println("  created backend/gomobile/notifier.go (generated)")
	}

	// Vendor the project so it builds offline and its deps — including the pinned
	// glaze fork — are committed in the project (surviving upstream changes),
	// matching goleo's own vendor-everything approach. Best-effort: on failure the
	// project simply fetches deps from the network on the first build instead.
	if replaceErr == nil {
		fmt.Println("  Vendoring dependencies (go mod vendor)...")
		vendor := exec.Command("go", "mod", "vendor")
		vendor.Dir = dir
		vendor.Stdout = os.Stdout
		vendor.Stderr = os.Stderr
		if err := vendor.Run(); err != nil {
			fmt.Printf("  Warning: go mod vendor failed (deps will be fetched on first build): %v\n", err)
		}
	}

	// Install frontend dependencies (vue, vite, @goleo/bridge) so the project is
	// runnable right away — like create-vite / Tauri. Skip with --no-install.
	if !newSkipInstall {
		installFrontendDeps(dir)
	}
	linkBridge(dir) // dev-only: override @goleo/bridge with a local checkout

	fmt.Println()
	fmt.Println("Project created successfully!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", name)
	if newSkipInstall {
		fmt.Println("  cd frontend && npm install && cd ..")
	}
	fmt.Println("  goleo dev          # Start development mode")
	fmt.Println("  goleo build        # Build for current platform")
	fmt.Println()

	return nil
}

// installFrontendDeps runs `npm install` in the scaffolded frontend so the
// project is ready to `goleo dev` immediately. Best-effort — a failure (offline,
// npm missing) just prints how to run it by hand.
func installFrontendDeps(projectDir string) {
	frontend := filepath.Join(projectDir, "frontend")
	if _, err := os.Stat(filepath.Join(frontend, "package.json")); err != nil {
		return
	}
	fmt.Println()
	fmt.Println("  Installing frontend dependencies (npm install)...")
	cmd := exec.Command("npm", "install")
	cmd.Dir = frontend
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("  Warning: npm install failed (%v)\n", err)
		fmt.Println("  Run it manually: cd frontend && npm install")
	}
}

func linkBridge(projectDir string) {
	// Dev-only: when working on goleo itself (GOLEO_ROOT set), link the local
	// @goleo/bridge so the scaffolded frontend uses your working copy. End users
	// get @goleo/bridge from npm via the frontend's package.json dependency —
	// nothing to link.
	goleoRoot := os.Getenv("GOLEO_ROOT")
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
