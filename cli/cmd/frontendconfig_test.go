package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGoleoJSON(t *testing.T, dir, frontendJSON string) {
	t.Helper()
	content := `{"version":"0.1.0","app_name":"Test",` + frontendJSON + `}`
	if err := os.WriteFile(filepath.Join(dir, "goleo.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFrontendConfigMissingFile(t *testing.T) {
	cfg := loadFrontendConfig(t.TempDir())
	if cfg.DevCommand != "" || cfg.DevPort != 0 {
		t.Errorf("expected zero value for missing goleo.json, got %+v", cfg)
	}
}

func TestLoadFrontendConfigNoFrontendSection(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "goleo.json"), []byte(`{"version":"0.1.0"}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := loadFrontendConfig(dir)
	if cfg.DevCommand != "" || cfg.DevPort != 0 {
		t.Errorf("expected zero value when frontend section absent, got %+v", cfg)
	}
}

func TestLoadFrontendConfigReadsDevCommandAndPort(t *testing.T) {
	dir := t.TempDir()
	writeGoleoJSON(t, dir, `"frontend":{"dev_command":"npm run dev","dev_port":5174}`)
	cfg := loadFrontendConfig(dir)
	if cfg.DevCommand != "npm run dev" {
		t.Errorf("DevCommand = %q, want %q", cfg.DevCommand, "npm run dev")
	}
	if cfg.DevPort != 5174 {
		t.Errorf("DevPort = %d, want 5174", cfg.DevPort)
	}
}

func TestLoadFrontendConfigIgnoresZeroOrNegativePort(t *testing.T) {
	dir := t.TempDir()
	writeGoleoJSON(t, dir, `"frontend":{"dev_command":"npm run dev","dev_port":0}`)
	cfg := loadFrontendConfig(dir)
	if cfg.DevPort != 0 {
		t.Errorf("DevPort = %d, want 0 (invalid port should not be trusted)", cfg.DevPort)
	}
}

func TestResolveDevServerFallsBackToViteWhenUnconfigured(t *testing.T) {
	projectDir := t.TempDir()
	frontendDir := t.TempDir()
	cmd, port := resolveDevServer(projectDir, frontendDir, 5173, "--host")
	if port != 5173 {
		t.Errorf("port = %d, want 5173 (default)", port)
	}
	if cmd.Dir != frontendDir {
		t.Errorf("cmd.Dir = %q, want frontendDir %q", cmd.Dir, frontendDir)
	}
	gotArgs := cmd.Args
	wantArgs := []string{"npx", "vite", "--port", "5173", "--host"}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", gotArgs, wantArgs)
	}
	for i := range wantArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Errorf("args[%d] = %q, want %q", i, gotArgs[i], wantArgs[i])
		}
	}
}

func TestResolveDevServerUsesCustomCommandWhenConfigured(t *testing.T) {
	projectDir := t.TempDir()
	frontendDir := t.TempDir()
	writeGoleoJSON(t, projectDir, `"frontend":{"dev_command":"npm run dev","dev_port":5174}`)

	cmd, port := resolveDevServer(projectDir, frontendDir, 5173, "--host")
	if port != 5174 {
		t.Errorf("port = %d, want 5174 (from goleo.json)", port)
	}
	if cmd.Dir != projectDir {
		t.Errorf("cmd.Dir = %q, want projectDir %q (not frontendDir)", cmd.Dir, projectDir)
	}
	// No intermediary shell, no extraViteArgs ("--host") leaking into a
	// custom command that isn't necessarily Vite at all.
	gotArgs := cmd.Args
	wantArgs := []string{"npm", "run", "dev"}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", gotArgs, wantArgs)
	}
	for i := range wantArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Errorf("args[%d] = %q, want %q", i, gotArgs[i], wantArgs[i])
		}
	}
}

func TestResolveDevServerIgnoresDevCommandWithoutDevPort(t *testing.T) {
	// Both fields are required together — a dev_command with no dev_port
	// leaves goleo with no way to know what port to wait on/report/reverse,
	// so it must fall back rather than guess.
	projectDir := t.TempDir()
	frontendDir := t.TempDir()
	writeGoleoJSON(t, projectDir, `"frontend":{"dev_command":"npm run dev"}`)

	cmd, port := resolveDevServer(projectDir, frontendDir, 5173)
	if port != 5173 {
		t.Errorf("port = %d, want 5173 (fallback, dev_port missing)", port)
	}
	if cmd.Args[0] != "npx" {
		t.Errorf("expected fallback to npx vite, got args %v", cmd.Args)
	}
}
