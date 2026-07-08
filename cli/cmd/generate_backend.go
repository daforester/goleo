package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// generateBackendEntrypoints (re)writes the Go entry points that glue a
// scaffolded project's backend/app package into each target — backend/main.go
// (desktop) and backend/gomobile/{gomobile.go,notifier.go} (mobile). These
// files are pure boilerplate with no per-app logic, so they are regenerated
// fresh before every goleo new/dev/build/emulate run instead of being
// hand-edited; all app-specific startup logic lives in backend/app/app.go.
func generateBackendEntrypoints(projectDir string) error {
	moduleName, err := parseModuleName(projectDir)
	if err != nil {
		return fmt.Errorf("resolving module name: %w", err)
	}
	cfg := projectConfig{ModuleName: moduleName}

	if err := os.MkdirAll(filepath.Join(projectDir, "backend", "gomobile"), 0755); err != nil {
		return fmt.Errorf("creating backend/gomobile: %w", err)
	}

	// gomobile.go's //go:embed all:frontend/dist requires this directory to
	// contain at least one file at compile time, even in dev mode where the
	// embedded copy is never actually served (Vite serves the frontend
	// directly). Without a placeholder here, a fresh clone or a
	// goleo emulate run that hasn't first run goleo build android/ios fails
	// with "pattern all:frontend/dist: no matching files found".
	gmDistDir := filepath.Join(projectDir, "backend", "gomobile", "frontend", "dist")
	if err := ensureEmbeddableDir(gmDistDir); err != nil {
		return fmt.Errorf("preparing backend/gomobile/frontend/dist: %w", err)
	}

	rendered := map[string]string{
		"backend/main.go":              tmplMainGo,
		"backend/gomobile/gomobile.go": tmplMobileGo,
	}
	for relPath, tmpl := range rendered {
		content, err := renderTemplate(tmpl, cfg)
		if err != nil {
			return fmt.Errorf("rendering %s: %w", relPath, err)
		}
		if err := os.WriteFile(filepath.Join(projectDir, relPath), []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", relPath, err)
		}
	}

	// notifier.go has no per-project placeholders — write it as-is.
	notifierPath := filepath.Join(projectDir, "backend", "gomobile", "notifier.go")
	if err := os.WriteFile(notifierPath, []byte(tmplMobileNotifierGo), 0644); err != nil {
		return fmt.Errorf("writing backend/gomobile/notifier.go: %w", err)
	}

	return nil
}

// ensureEmbeddableDir makes sure dir exists and contains at least one file,
// so a //go:embed pattern rooted there always resolves. It never touches an
// existing non-empty directory, so real frontend build output copied in by
// goleo build android/ios (see copyDir call sites in build.go) is untouched.
func ensureEmbeddableDir(dir string) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".gitkeep"), nil, 0644)
}
