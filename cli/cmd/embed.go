package cmd

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates
var mobileTemplates embed.FS

type mobileConfig struct {
	PackageName string
	AppName     string
	DevPort     int
	HasIcon     bool // a bundle.icon source resolved → manifest/xcodegen wire it in
}

func loadMobileConfig(projectDir string) mobileConfig {
	cfg := mobileConfig{
		PackageName: "com.goleo.app",
		AppName:     "Goleo App",
		DevPort:     5173,
	}
	data, err := os.ReadFile(filepath.Join(projectDir, "goleo.json"))
	if err != nil {
		return cfg
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg
	}
	if name, ok := raw["app_name"].(string); ok && name != "" {
		cfg.AppName = name
	}
	if mobile, ok := raw["mobile"].(map[string]any); ok {
		if android, ok := mobile["android"].(map[string]any); ok {
			if pkg, ok := android["package_name"].(string); ok && pkg != "" {
				cfg.PackageName = pkg
			}
		}
	}
	return cfg
}

func extractMobileTemplate(templateDir, outputDir string, cfg *mobileConfig) error {
	if cfg == nil {
		defaultCfg := loadMobileConfig(".")
		cfg = &defaultCfg
	}

	entries := mobileTemplates

	// Try the mode-specific template dir first, fall back to generic
	templatePath := "templates/" + templateDir
	if _, err := entries.ReadDir(templatePath); err != nil {
		// Fall back to plain template name (for production: "android")
		templatePath = "templates/" + templateDir
	}

	err := fs.WalkDir(entries, templatePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(templatePath, path)
		if rel == "" {
			return nil
		}

		// Replace package path in relative path
		pkgPath := strings.ReplaceAll(cfg.PackageName, ".", string(filepath.Separator))
		rel = strings.ReplaceAll(rel, "com/goleo/app", pkgPath)

		target := filepath.Join(outputDir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		data, err := entries.ReadFile(path)
		if err != nil {
			return err
		}

		// Process through Go template
		tmpl, err := template.New("").Parse(string(data))
		if err != nil {
			// If template parsing fails, write as-is
			return os.WriteFile(target, data, 0644)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, cfg); err != nil {
			return os.WriteFile(target, data, 0644)
		}

		return os.WriteFile(target, buf.Bytes(), 0644)
	})
	return err
}
