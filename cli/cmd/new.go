package cmd

import (
	"bytes"
	"fmt"
	"os"
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

	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0755); err != nil {
		return fmt.Errorf("failed to create backend dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "frontend", "src"), 0755); err != nil {
		return fmt.Errorf("failed to create frontend dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "frontend", "public"), 0755); err != nil {
		return fmt.Errorf("failed to create public dir: %w", err)
	}

	files := map[string]string{
		"backend/main.go":     tmplMainGo,
		"backend/commands.go": tmplCommandsGo,
		"backend/go.mod":      tmplGoMod,
		"frontend/package.json": tmplFrontendPackageJSON,
		"frontend/index.html":   tmplIndexHTML,
		"frontend/vite.config.ts": tmplViteConfig,
		"frontend/tsconfig.json":  tmplTsconfig,
		"frontend/env.d.ts":       tmplEnvDTS,
		"frontend/src/main.ts":    tmplMainTS,
		"frontend/src/App.vue":    tmplAppVue,
		"frontend/src/style.css":  tmplStyleCSS,
		"package.json":            tmplRootPackageJSON,
		"goleo.json":              tmplGoleoJSON,
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
	fmt.Println("Project created successfully!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", name)
	fmt.Println("  cd frontend && npm install")
	fmt.Println("  cd ..")
	fmt.Println("  goleo dev          # Start development mode")
	fmt.Println("  goleo build        # Build for current platform")
	fmt.Println()

	return nil
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


