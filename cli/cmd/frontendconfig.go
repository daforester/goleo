package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// frontendConfig is goleo.json's "frontend" section, the parts relevant to
// how the dev server gets started. The scaffold template
// (templates.go's "dev_command": "npm run dev") has written this key into
// every project's goleo.json since day one, but nothing ever read it back —
// dev.go/emulate.go hardcoded `npx vite` regardless. This makes it real.
type frontendConfig struct {
	// DevCommand overrides goleo's built-in `npx vite` invocation — for a
	// frontend whose own dev server wraps Vite (or doesn't use it at all),
	// e.g. Nuxt's `nuxt dev`. Run as-is from the project root: once set, the
	// project owns its dev server's full configuration (host, HMR, env,
	// port binding, ...) — goleo appends no flags and sets no env vars.
	DevCommand string
	// DevPort is the port DevCommand's server binds. Required alongside
	// DevCommand — goleo has no other way to know what port to wait on,
	// report, or adb-reverse.
	DevPort int
	// BuildCommand overrides goleo's built-in `npx vite build` invocation —
	// for a frontend whose own build wraps Vite (or doesn't use it at all),
	// e.g. Nuxt's `nuxt generate`. Run as-is from the project root.
	// Independent of DevCommand/DevPort — a project can override one without
	// the other.
	BuildCommand string
	// DistDir overrides the "dist" subdirectory name goleo looks for the
	// built frontend in (relative to the frontend directory). Nuxt's
	// `nuxt generate` writes to ".output/public", not "dist".
	DistDir string
	// Directory is the frontend's location relative to the project root — the
	// default for --frontend-dir. The scaffold has written this key since day
	// one ("frontend"), but only the flag was ever read: a project whose
	// frontend sits elsewhere had to repeat -f on every invocation, and
	// `emulate` / `dev pwa`, which expose no such flag, could not be pointed
	// at it at all.
	Directory string
}

// loadFrontendConfig reads goleo.json's "frontend" section, following the
// exact tolerant pattern of loadMobileConfig/loadBundleConfig (embed.go/
// bundle.go): a missing file, invalid JSON, or missing keys all just mean
// "not configured", not an error — most projects don't set these and must
// fall back to goleo's built-in Vite invocation completely unchanged.
func loadFrontendConfig(projectDir string) frontendConfig {
	var cfg frontendConfig
	data, err := os.ReadFile(filepath.Join(projectDir, "goleo.json"))
	if err != nil {
		return cfg
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg
	}
	frontend, ok := raw["frontend"].(map[string]any)
	if !ok {
		return cfg
	}
	if cmd, ok := frontend["dev_command"].(string); ok {
		cfg.DevCommand = cmd
	}
	if port, ok := frontend["dev_port"].(float64); ok && port > 0 {
		cfg.DevPort = int(port)
	}
	if cmd, ok := frontend["build_command"].(string); ok {
		cfg.BuildCommand = cmd
	}
	if dir, ok := frontend["dist_dir"].(string); ok {
		cfg.DistDir = dir
	}
	if dir, ok := frontend["directory"].(string); ok {
		cfg.Directory = dir
	}
	return cfg
}

// resolveFrontendDir returns the frontend directory a command should use:
// --frontend-dir when the caller actually passed it, else goleo.json's
// frontend.directory, else the flag's own default. cmd is consulted with
// Changed() rather than comparing against the default, so an explicit
// `-f frontend` still wins over a config that says something else.
func resolveFrontendDir(cmd *cobra.Command, flagValue, projectDir string) string {
	if cmd != nil && cmd.Flags().Changed("frontend-dir") {
		return flagValue
	}
	if dir := loadFrontendConfig(projectDir).Directory; dir != "" {
		return dir
	}
	return flagValue
}

// resolveDevServer returns the command to start the frontend dev server and
// the port it will bind. Reads goleo.json's frontend.dev_command/dev_port
// from projectDir (where goleo.json lives — not necessarily frontendDir;
// `goleo emulate` runs its built-in Vite from a project-root-relative
// "frontend" shim directory that has no goleo.json of its own).
//
// When both dev_command and dev_port are set, splits the command on
// whitespace and execs it directly from projectDir — no intermediary shell:
// every real dev_command here is a plain space-separated invocation like
// "npm run dev", so a shell buys nothing but an extra process and an
// assumption that sh/cmd is present. Returns dev_port. extraViteArgs and any
// env vars the caller layers on afterward are for the built-in-Vite fallback
// only, since a custom command isn't necessarily Vite at all. Otherwise
// falls back to goleo's existing
// `npx vite --port <defaultPort> <extraViteArgs...>` from frontendDir,
// exactly as before — fully backward compatible with every project that
// doesn't set these fields.
func resolveDevServer(projectDir, frontendDir string, defaultPort int, extraViteArgs ...string) (*exec.Cmd, int) {
	cfg := loadFrontendConfig(projectDir)
	if cfg.DevCommand != "" && cfg.DevPort > 0 {
		if parts := strings.Fields(cfg.DevCommand); len(parts) > 0 {
			cmd := exec.Command(parts[0], parts[1:]...)
			cmd.Dir = projectDir
			return cmd, cfg.DevPort
		}
	}

	args := append([]string{"vite", "--port", fmt.Sprintf("%d", defaultPort)}, extraViteArgs...)
	cmd := exec.Command("npx", args...)
	cmd.Dir = frontendDir
	return cmd, defaultPort
}
