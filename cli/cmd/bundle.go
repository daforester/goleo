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
	Description string // one-line app description (installers + exe version info)
	Copyright  string // e.g. "© 2026 Example Ltd" (exe version info, .deb)
	Category   string // freedesktop/macOS category (e.g. "Utility")
	Homepage   string // project/homepage URL

	Icon     string // single source icon (PNG); platform variants derived from it
	IconICO  string // Windows icon path (overrides Icon)
	IconICNS string // macOS icon path (overrides Icon)
	IconPNG  string // Linux icon path (overrides Icon)

	// Publish (updater manifest)
	UpdateURLBase string // base URL where update artifacts are hosted
	ReleaseNotes  string // notes for this release

	URLScheme string // custom URL scheme to register (deep links), e.g. "myapp"
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
		cfg.Description = str("description")
		cfg.Copyright = str("copyright")
		cfg.Category = str("category")
		cfg.Homepage = str("homepage")
		cfg.Icon = str("icon")
		cfg.IconICO = str("icon_ico")
		cfg.IconICNS = str("icon_icns")
		cfg.IconPNG = str("icon_png")
		cfg.UpdateURLBase = str("update_url_base")
		cfg.ReleaseNotes = str("release_notes")
		cfg.URLScheme = str("url_scheme")
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
	sc := loadSignConfig()
	switch target.GOOS {
	case "windows":
		return bundleWindows(binaryPath, cfg, outDir, sc)
	case "darwin":
		return bundleDarwin(binaryPath, cfg, outDir, sc)
	case "linux":
		return bundleLinux(binaryPath, cfg, outDir, sc)
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

func bundleWindows(binaryPath string, cfg bundleConfig, outDir string, sc signConfig) error {
	tool, err := ensureMakensis() // detects makensis; auto-installs NSIS if missing
	if err != nil {
		return err
	}
	// Sign the app binary first so the installed executable is trusted.
	if err := signWindows(sc, binaryPath); err != nil {
		return err
	}
	binBase := filepath.Base(binaryPath)
	// NOTE: makensis cd's into the script's directory, so every path referenced by
	// the .nsi (OutFile, File) MUST be absolute — otherwise a relative OutFile
	// under outDir resolves to outDir/outDir (the dist\bundle\dist\bundle bug).
	outFile, _ := filepath.Abs(filepath.Join(outDir, installerName(cfg, ".exe", "-setup")))
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
	// Sign the installer itself.
	if err := signWindows(sc, outFile); err != nil {
		return err
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
	// Installer metadata (Details tab of the setup .exe).
	fmt.Fprintf(&b, "VIProductVersion %q\n", to4PartVersion(cfg.Version))
	fmt.Fprintf(&b, "VIAddVersionKey \"ProductName\" %q\n", cfg.AppName)
	fmt.Fprintf(&b, "VIAddVersionKey \"ProductVersion\" %q\n", cfg.Version)
	fmt.Fprintf(&b, "VIAddVersionKey \"FileVersion\" %q\n", cfg.Version) // NSIS requires this alongside VIProductVersion
	if cfg.Description != "" {
		fmt.Fprintf(&b, "VIAddVersionKey \"FileDescription\" %q\n", cfg.Description)
	}
	if cfg.Publisher != "" {
		fmt.Fprintf(&b, "VIAddVersionKey \"CompanyName\" %q\n", cfg.Publisher)
		fmt.Fprintf(&b, "BrandingText %q\n", cfg.Publisher)
	}
	if cfg.Copyright != "" {
		fmt.Fprintf(&b, "VIAddVersionKey \"LegalCopyright\" %q\n", cfg.Copyright)
	}
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

func bundleDarwin(binaryPath string, cfg bundleConfig, outDir string, sc signConfig) error {
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
	// Icon: an explicit .icns wins; otherwise derive one from the single source PNG.
	if cfg.IconICNS != "" {
		if _, err := os.Stat(cfg.IconICNS); err == nil {
			copyFile(cfg.IconICNS, filepath.Join(resources, "icon.icns"))
		}
	} else if src, ok := resolveSourceIcon(cfg); ok {
		if img, err := loadPNG(src); err == nil {
			if err := writeICNS(img, filepath.Join(resources, "icon.icns")); err != nil {
				fmt.Println("  Warning: could not generate .icns:", err)
			} else {
				fmt.Println("  Generated icon.icns from bundle.icon")
			}
		}
	}
	plist := infoPlist(cfg, binBase)
	if err := os.WriteFile(filepath.Join(appDir, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		return err
	}
	fmt.Printf("  Created %s\n", appDir)

	// Code-sign the .app (hardened runtime) before packaging it into the .dmg.
	if err := codesignMac(sc, appDir); err != nil {
		return err
	}

	// The .dmg needs hdiutil, which is macOS-only.
	tool, err := requireTool("hdiutil", "part of macOS; run this on macOS")
	if err != nil {
		fmt.Printf("  Skipping .dmg: %v\n", err)
		return nil
	}
	dmg, _ := filepath.Abs(filepath.Join(outDir, installerName(cfg, ".dmg", "")))
	os.Remove(dmg)
	cmd := exec.Command(tool, "create", "-volname", cfg.AppName, "-srcfolder", appDir, "-ov", "-format", "UDZO", dmg)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bundle: hdiutil failed: %w", err)
	}
	fmt.Printf("  Created %s\n", dmg)
	// Notarize + staple the .dmg (Apple requires notarization for distribution).
	if err := notarizeMac(sc, dmg); err != nil {
		return err
	}
	return nil
}

func infoPlist(cfg bundleConfig, binBase string) string {
	urlTypes := ""
	if cfg.URLScheme != "" {
		urlTypes = fmt.Sprintf(`	<key>CFBundleURLTypes</key>
	<array><dict>
		<key>CFBundleURLName</key><string>%s</string>
		<key>CFBundleURLSchemes</key><array><string>%s</string></array>
	</dict></array>
`, cfg.Identifier, cfg.URLScheme)
	}
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
%s</dict>
</plist>
`, cfg.AppName, binBase, cfg.Identifier, cfg.Version, cfg.Version, urlTypes)
}

// --- Linux (nfpm → .deb/.rpm) ---

func bundleLinux(binaryPath string, cfg bundleConfig, outDir string, sc signConfig) error {
	_ = sc // Linux package signing (dpkg-sig / rpm --addsign, GPG) is a follow-up
	tool, err := requireTool("nfpm", "go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest")
	if err != nil {
		return err
	}
	binBase := filepath.Base(binaryPath)

	// Desktop integration: install a hicolor icon (from bundle.icon_png or derived
	// from the single bundle.icon) plus a .desktop launcher entry.
	iconPath := ""
	if cfg.IconPNG != "" {
		if _, err := os.Stat(cfg.IconPNG); err == nil {
			iconPath, _ = filepath.Abs(cfg.IconPNG)
		}
	} else if src, ok := resolveSourceIcon(cfg); ok {
		if img, err := loadPNG(src); err == nil {
			gen := filepath.Join(outDir, slug(cfg.AppName)+".png")
			if err := writeResizedPNG(img, 256, gen); err == nil {
				iconPath, _ = filepath.Abs(gen)
			}
		}
	}
	desktopPath := filepath.Join(outDir, slug(cfg.AppName)+".desktop")
	if err := os.WriteFile(desktopPath, []byte(desktopEntry(cfg, binBase)), 0o644); err != nil {
		return err
	}
	desktopPath, _ = filepath.Abs(desktopPath)

	nfpmYAML := nfpmConfig(cfg, binaryPath, binBase, iconPath, desktopPath)
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

func nfpmConfig(cfg bundleConfig, binaryPath, binBase, iconPath, desktopPath string) string {
	maintainer := cfg.Publisher
	if maintainer == "" {
		maintainer = "unknown <noreply@example.com>"
	}
	desc := cfg.Description
	if desc == "" {
		desc = cfg.AppName
	}
	extra := ""
	if cfg.Homepage != "" {
		extra += fmt.Sprintf("homepage: %q\n", cfg.Homepage)
	}
	if cfg.Category != "" {
		extra += fmt.Sprintf("section: %q\n", cfg.Category)
	}
	contents := fmt.Sprintf(`  - src: "%s"
    dst: "/usr/bin/%s"
`, filepath.ToSlash(binaryPath), binBase)
	if desktopPath != "" {
		contents += fmt.Sprintf(`  - src: "%s"
    dst: "/usr/share/applications/%s.desktop"
`, filepath.ToSlash(desktopPath), slug(cfg.AppName))
	}
	if iconPath != "" {
		contents += fmt.Sprintf(`  - src: "%s"
    dst: "/usr/share/icons/hicolor/256x256/apps/%s.png"
`, filepath.ToSlash(iconPath), slug(cfg.AppName))
	}
	return fmt.Sprintf(`name: "%s"
arch: "amd64"
version: "%s"
maintainer: "%s"
description: "%s"
%scontents:
%s`, slug(cfg.AppName), cfg.Version, maintainer, desc, extra, contents)
}

// desktopEntry builds a freedesktop .desktop launcher for the installed binary.
func desktopEntry(cfg bundleConfig, binBase string) string {
	categories := cfg.Category
	if categories == "" {
		categories = "Utility"
	}
	entry := "[Desktop Entry]\n" +
		"Type=Application\n" +
		fmt.Sprintf("Name=%s\n", cfg.AppName) +
		fmt.Sprintf("Exec=/usr/bin/%s\n", binBase) +
		fmt.Sprintf("Icon=%s\n", slug(cfg.AppName)) +
		"Terminal=false\n" +
		fmt.Sprintf("Categories=%s;\n", categories)
	if cfg.Description != "" {
		entry += fmt.Sprintf("Comment=%s\n", cfg.Description)
	}
	return entry
}

// slug lowercases and hyphenates an app name for use in filenames/package names.
// installerName is the installer file name: the -o value if given (its base name,
// minus any extension), otherwise "<slug>-<version>"; plus suffix and ext. The
// suffix keeps the installer distinct from the app binary (e.g. myapp.exe vs
// myapp-setup.exe) so -o can't make them collide.
func installerName(cfg bundleConfig, ext, suffix string) string {
	base := fmt.Sprintf("%s-%s", slug(cfg.AppName), cfg.Version)
	if buildOutput != "" {
		b := filepath.Base(buildOutput)
		base = strings.TrimSuffix(b, filepath.Ext(b))
	}
	return base + suffix + ext
}

// to4PartVersion coerces a version like "1.2.3" into the "1.2.3.0" 4-part form
// NSIS's VIProductVersion requires; non-numeric characters are dropped per part.
func to4PartVersion(v string) string {
	parts := strings.Split(v, ".")
	out := make([]string, 4)
	for i := range out {
		out[i] = "0"
		if i < len(parts) {
			n := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return r
				}
				return -1
			}, parts[i])
			if n != "" {
				out[i] = n
			}
		}
	}
	return strings.Join(out, ".")
}

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
