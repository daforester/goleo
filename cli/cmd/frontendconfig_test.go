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

func TestLoadFrontendConfigReadsBuildCommandAndDistDir(t *testing.T) {
	dir := t.TempDir()
	writeGoleoJSON(t, dir, `"frontend":{"build_command":"npm run build","dist_dir":".output/public"}`)
	cfg := loadFrontendConfig(dir)
	if cfg.BuildCommand != "npm run build" {
		t.Errorf("BuildCommand = %q, want %q", cfg.BuildCommand, "npm run build")
	}
	if cfg.DistDir != ".output/public" {
		t.Errorf("DistDir = %q, want %q", cfg.DistDir, ".output/public")
	}
}

func TestLoadFrontendConfigBuildCommandAndDevCommandAreIndependent(t *testing.T) {
	// Unlike dev_command/dev_port, build_command/dist_dir don't require each
	// other, and neither pair requires the other — a project can set just
	// one of the four fields.
	dir := t.TempDir()
	writeGoleoJSON(t, dir, `"frontend":{"build_command":"npm run build"}`)
	cfg := loadFrontendConfig(dir)
	if cfg.BuildCommand != "npm run build" {
		t.Errorf("BuildCommand = %q, want %q", cfg.BuildCommand, "npm run build")
	}
	if cfg.DistDir != "" {
		t.Errorf("DistDir = %q, want empty (not set)", cfg.DistDir)
	}
	if cfg.DevCommand != "" || cfg.DevPort != 0 {
		t.Errorf("dev fields should be untouched, got DevCommand=%q DevPort=%d", cfg.DevCommand, cfg.DevPort)
	}
}

func TestResolveBuildCommandFallsBackToViteWhenUnconfigured(t *testing.T) {
	projectDir := t.TempDir()
	frontendDir := t.TempDir()
	cmd, isCustom := resolveBuildCommand(projectDir, frontendDir, []string{"FOO=bar"})
	if isCustom {
		t.Error("isCustom = true, want false (no build_command set)")
	}
	if cmd.Dir != frontendDir {
		t.Errorf("cmd.Dir = %q, want frontendDir %q", cmd.Dir, frontendDir)
	}
	wantArgs := []string{"npx", "vite", "build"}
	if len(cmd.Args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", cmd.Args, wantArgs)
	}
	for i := range wantArgs {
		if cmd.Args[i] != wantArgs[i] {
			t.Errorf("args[%d] = %q, want %q", i, cmd.Args[i], wantArgs[i])
		}
	}
}

func TestResolveBuildCommandUsesCustomCommandWhenConfigured(t *testing.T) {
	projectDir := t.TempDir()
	frontendDir := t.TempDir()
	writeGoleoJSON(t, projectDir, `"frontend":{"build_command":"npm run build"}`)

	cmd, isCustom := resolveBuildCommand(projectDir, frontendDir, nil)
	if !isCustom {
		t.Error("isCustom = false, want true (build_command set)")
	}
	if cmd.Dir != frontendDir {
		t.Errorf("cmd.Dir = %q, want frontendDir %q", cmd.Dir, frontendDir)
	}
	wantArgs := []string{"npm", "run", "build"}
	if len(cmd.Args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", cmd.Args, wantArgs)
	}
	for i := range wantArgs {
		if cmd.Args[i] != wantArgs[i] {
			t.Errorf("args[%d] = %q, want %q", i, cmd.Args[i], wantArgs[i])
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
