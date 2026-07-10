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

	// main.go and gomobile.go's //go:embed all:frontend/dist directives
	// require their respective directories to contain at least one file at
	// compile time, even in dev mode where the embedded copy is never
	// actually served (Vite serves the frontend directly over HTTP).
	// Without a placeholder here, a fresh clone or a goleo dev/emulate run
	// that hasn't first run goleo build fails with
	// "pattern all:frontend/dist: no matching files found".
	if err := os.MkdirAll(filepath.Join(projectDir, "backend"), 0755); err != nil {
		return fmt.Errorf("creating backend: %w", err)
	}
	desktopDistDir := filepath.Join(projectDir, "backend", "frontend", "dist")
	if err := ensureEmbeddableDir(desktopDistDir); err != nil {
		return fmt.Errorf("preparing backend/frontend/dist: %w", err)
	}
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

	// notifier.go and the feature provider files have no per-project
	// placeholders — write them as-is. Each feature file is its own
	// goleo_*-gated file (mirroring runtime/*_reexport.go's own build tags)
	// so that disabling a feature in app.go — which drops its tag from
	// mobileBindTags — excludes the matching file too, instead of leaving a
	// dangling reference to a runtime symbol that no longer exists.
	static := map[string]string{
		"backend/gomobile/notifier.go":   tmplMobileNotifierGo,
		"backend/gomobile/features.go":   tmplMobileFeaturesGo,
		"backend/gomobile/clipboard.go":  tmplMobileClipboardGo,
		"backend/gomobile/battery.go":    tmplMobileBatteryGo,
		"backend/gomobile/wakelock.go":   tmplMobileWakeLockGo,
		"backend/gomobile/sensors.go":    tmplMobileSensorsGo,
		"backend/gomobile/background.go": tmplMobileBackgroundGo,
		"backend/gomobile/nfc.go":        tmplMobileNFCGo,
		"backend/gomobile/ble.go":        tmplMobileBLEGo,
	}
	for relPath, content := range static {
		if err := os.WriteFile(filepath.Join(projectDir, relPath), []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", relPath, err)
		}
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
