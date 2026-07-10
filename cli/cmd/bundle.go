package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// bundleConfig drives installer packaging. Loaded from goleo.json's top-level
// app_name/version plus an optional "bundle" object.
type bundleConfig struct {
	AppName    string
	Version    string
	Identifier string // reverse-DNS, e.g. com.example.app
	Publisher  string
	IconICO    string // Windows icon path
	IconICNS   string // macOS icon path
	IconPNG    string // Linux icon path
}

func loadBundleConfig(projectDir string) bundleConfig {
	cfg := bundleConfig{
		AppName:    "Goleo App",
		Version:    "0.1.0",
		Identifier: "com.goleo.app",
	}
	data, err := os.ReadFile(filepath.Join(projectDir, "goleo.json"))
	if err != nil {
		return cfg
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg
	}
	if v, ok := raw["app_name"].(string); ok && v != "" {
		cfg.AppName = v
	}
	if v, ok := raw["version"].(string); ok && v != "" {
		cfg.Version = v
	}
	if b, ok := raw["bundle"].(map[string]any); ok {
		str := func(k string) string { s, _ := b[k].(string); return s }
		if v := str("identifier"); v != "" {
			cfg.Identifier = v
		}
		cfg.Publisher = str("publisher")
		cfg.IconICO = str("icon_ico")
		cfg.IconICNS = str("icon_icns")
		cfg.IconPNG = str("icon_png")
	}
	return cfg
}

// runBundle packages an already-built desktop binary into a native installer
// for target.GOOS, emitting into dist/bundle/. External packaging tools are
// detected first; a missing tool produces a clear, non-fatal-to-diagnose error
// with an install hint rather than a cryptic failure.
func runBundle(target buildTarget, binaryPath string, cfg bundleConfig) error {
	outDir := filepath.Join("dist", "bundle")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("bundle: built binary not found at %s: %w", binaryPath, err)
	}

	fmt.Printf("\n  Bundling %s installer...\n", target.Label)
	switch target.GOOS {
	case "windows":
		return bundleWindows(binaryPath, cfg, outDir)
	case "darwin":
		return bundleDarwin(binaryPath, cfg, outDir)
	case "linux":
		return bundleLinux(binaryPath, cfg, outDir)
	default:
		return fmt.Errorf("bundle: unsupported target %s (desktop only)", target.GOOS)
	}
}

func requireTool(name, hint string) (string, error) {
	if path, ok := findTool(name); ok {
		return path, nil
	}
	return "", fmt.Errorf("bundle: %q not found — install it (%s) and retry", name, hint)
}

// --- Windows (NSIS) ---

func bundleWindows(binaryPath string, cfg bundleConfig, outDir string) error {
	tool, err := requireTool("makensis", "https://nsis.sourceforge.io")
	if err != nil {
		return err
	}
	binBase := filepath.Base(binaryPath)
	outFile := filepath.Join(outDir, fmt.Sprintf("%s-%s-setup.exe", slug(cfg.AppName), cfg.Version))
	nsi := nsisScript(cfg, binaryPath, binBase, outFile)

	scriptPath := filepath.Join(outDir, "installer.nsi")
	if err := os.WriteFile(scriptPath, []byte(nsi), 0o644); err != nil {
		return err
	}
	cmd := exec.Command(tool, scriptPath)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bundle: makensis failed: %w", err)
	}
	fmt.Printf("  Created %s\n", outFile)
	return nil
}

func nsisScript(cfg bundleConfig, binaryPath, binBase, outFile string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Name %q\n", cfg.AppName)
	fmt.Fprintf(&b, "OutFile %q\n", outFile)
	fmt.Fprintf(&b, "InstallDir \"$PROGRAMFILES64\\%s\"\n", cfg.AppName)
	fmt.Fprintf(&b, "RequestExecutionLevel admin\n")
	b.WriteString("Page directory\nPage instfiles\nUninstPage uninstConfirm\nUninstPage instfiles\n")
	b.WriteString("Section\n  SetOutPath $INSTDIR\n")
	fmt.Fprintf(&b, "  File %q\n", binaryPath)
	fmt.Fprintf(&b, "  CreateShortcut \"$SMPROGRAMS\\%s.lnk\" \"$INSTDIR\\%s\"\n", cfg.AppName, binBase)
	b.WriteString("  WriteUninstaller \"$INSTDIR\\uninstall.exe\"\nSectionEnd\n")
	b.WriteString("Section \"Uninstall\"\n")
	fmt.Fprintf(&b, "  Delete \"$INSTDIR\\%s\"\n", binBase)
	b.WriteString("  Delete \"$INSTDIR\\uninstall.exe\"\n")
	fmt.Fprintf(&b, "  Delete \"$SMPROGRAMS\\%s.lnk\"\n", cfg.AppName)
	b.WriteString("  RMDir \"$INSTDIR\"\nSectionEnd\n")
	return b.String()
}

// --- macOS (.app bundle + .dmg) ---

func bundleDarwin(binaryPath string, cfg bundleConfig, outDir string) error {
	// The .app bundle is pure file operations (no external tool).
	appDir := filepath.Join(outDir, cfg.AppName+".app")
	os.RemoveAll(appDir)
	macOS := filepath.Join(appDir, "Contents", "MacOS")
	resources := filepath.Join(appDir, "Contents", "Resources")
	if err := os.MkdirAll(macOS, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(resources, 0o755); err != nil {
		return err
	}
	binBase := filepath.Base(binaryPath)
	if err := copyFile(binaryPath, filepath.Join(macOS, binBase)); err != nil {
		return err
	}
	os.Chmod(filepath.Join(macOS, binBase), 0o755)
	if cfg.IconICNS != "" {
		if _, err := os.Stat(cfg.IconICNS); err == nil {
			copyFile(cfg.IconICNS, filepath.Join(resources, "icon.icns"))
		}
	}
	plist := infoPlist(cfg, binBase)
	if err := os.WriteFile(filepath.Join(appDir, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		return err
	}
	fmt.Printf("  Created %s\n", appDir)

	// The .dmg needs hdiutil, which is macOS-only.
	tool, err := requireTool("hdiutil", "part of macOS; run this on macOS")
	if err != nil {
		fmt.Printf("  Skipping .dmg: %v\n", err)
		return nil
	}
	dmg := filepath.Join(outDir, fmt.Sprintf("%s-%s.dmg", slug(cfg.AppName), cfg.Version))
	os.Remove(dmg)
	cmd := exec.Command(tool, "create", "-volname", cfg.AppName, "-srcfolder", appDir, "-ov", "-format", "UDZO", dmg)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bundle: hdiutil failed: %w", err)
	}
	fmt.Printf("  Created %s\n", dmg)
	return nil
}

func infoPlist(cfg bundleConfig, binBase string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>%s</string>
	<key>CFBundleExecutable</key><string>%s</string>
	<key>CFBundleIdentifier</key><string>%s</string>
	<key>CFBundleVersion</key><string>%s</string>
	<key>CFBundleShortVersionString</key><string>%s</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleIconFile</key><string>icon.icns</string>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
`, cfg.AppName, binBase, cfg.Identifier, cfg.Version, cfg.Version)
}

// --- Linux (nfpm → .deb/.rpm) ---

func bundleLinux(binaryPath string, cfg bundleConfig, outDir string) error {
	tool, err := requireTool("nfpm", "go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest")
	if err != nil {
		return err
	}
	binBase := filepath.Base(binaryPath)
	nfpmYAML := nfpmConfig(cfg, binaryPath, binBase)
	cfgPath := filepath.Join(outDir, "nfpm.yaml")
	if err := os.WriteFile(cfgPath, []byte(nfpmYAML), 0o644); err != nil {
		return err
	}
	for _, packager := range []string{"deb", "rpm"} {
		cmd := exec.Command(tool, "package", "--config", cfgPath, "--packager", packager, "--target", outDir)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("bundle: nfpm %s failed: %w", packager, err)
		}
		fmt.Printf("  Created %s package in %s\n", packager, outDir)
	}
	// AppImage (appimagetool) is a further target; left for a follow-up.
	return nil
}

func nfpmConfig(cfg bundleConfig, binaryPath, binBase string) string {
	maintainer := cfg.Publisher
	if maintainer == "" {
		maintainer = "unknown <noreply@example.com>"
	}
	return fmt.Sprintf(`name: "%s"
arch: "amd64"
version: "%s"
maintainer: "%s"
description: "%s"
contents:
  - src: "%s"
    dst: "/usr/bin/%s"
`, slug(cfg.AppName), cfg.Version, maintainer, cfg.AppName, binaryPath, binBase)
}

// slug lowercases and hyphenates an app name for use in filenames/package names.
func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteRune('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
