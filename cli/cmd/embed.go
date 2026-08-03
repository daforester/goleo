package cmd

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
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

	// VersionName is the user-visible version (Android versionName /
	// CFBundleShortVersionString), from goleo.json's top-level "version".
	VersionName string
	// VersionCode is the integer build number (Android versionCode /
	// CFBundleVersion). Play requires it to increase on every upload.
	VersionCode int
	MinSDK      int
	TargetSDK   int
	// ExtraPermissions are appended to the generated Android manifest verbatim.
	ExtraPermissions []string

	// IOSBundleID is the iOS bundle identifier. It falls back to PackageName,
	// which used to be the *only* source — so setting mobile.ios.bundle_identifier
	// in goleo.json did nothing and every iOS build reused the Android id.
	IOSBundleID string
	// IOSDeploymentTarget is the minimum iOS version.
	IOSDeploymentTarget string
}

// Defaults for the mobile toolchain. These mirror what the templates hardcoded
// before goleo.json's mobile section was actually read.
const (
	defaultAndroidMinSDK    = 24
	defaultAndroidTargetSDK = 36
	defaultIOSDeployTarget  = "15.0"
)

func loadMobileConfig(projectDir string) mobileConfig {
	cfg := mobileConfig{
		PackageName:         "com.goleo.app",
		AppName:             "Goleo App",
		DevPort:             5173,
		VersionName:         "1.0",
		VersionCode:         1,
		MinSDK:              defaultAndroidMinSDK,
		TargetSDK:           defaultAndroidTargetSDK,
		IOSDeploymentTarget: defaultIOSDeployTarget,
	}
	raw := loadGoleoJSON(projectDir)
	if raw.AppName != "" {
		cfg.AppName = raw.AppName
	}
	if raw.Mobile.Android.PackageName != "" {
		cfg.PackageName = raw.Mobile.Android.PackageName
	}
	if raw.Mobile.Android.MinSDK > 0 {
		cfg.MinSDK = raw.Mobile.Android.MinSDK
	}
	if raw.Mobile.Android.TargetSDK > 0 {
		cfg.TargetSDK = raw.Mobile.Android.TargetSDK
	}
	cfg.ExtraPermissions = raw.Mobile.Android.ExtraPermissions

	// Version: goleo.json's top-level "version" drives versionName, and
	// versionCode is derived from it unless explicitly overridden.
	if raw.Version != "" {
		cfg.VersionName = raw.Version
		if code, ok := versionCodeFromSemver(raw.Version); ok {
			cfg.VersionCode = code
		}
	}
	if raw.Mobile.Android.VersionCode > 0 {
		cfg.VersionCode = raw.Mobile.Android.VersionCode
	}

	cfg.IOSBundleID = raw.Mobile.IOS.BundleIdentifier
	if cfg.IOSBundleID == "" {
		cfg.IOSBundleID = cfg.PackageName
	}
	if raw.Mobile.IOS.DeploymentTarget != "" {
		cfg.IOSDeploymentTarget = raw.Mobile.IOS.DeploymentTarget
	}
	return cfg
}

// versionCodeFromSemver derives a monotonically increasing Android versionCode
// from a semver string: 1.2.3 → 10203. Pre-release/build suffixes are ignored, so
// 1.2.3-rc1 and 1.2.3 collide — override with mobile.android.version_code (or the
// CI env var) when that matters.
func versionCodeFromSemver(v string) (int, bool) {
	var major, minor, patch int
	// Tolerate a leading "v" and any -prerelease/+build suffix.
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return 0, false
	}
	nums := []*int{&major, &minor, &patch}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, false
		}
		*nums[i] = n
	}
	if minor > 99 || patch > 99 {
		// Would overflow the *100 packing and could go backwards.
		return 0, false
	}
	return major*10000 + minor*100 + patch, true
}

// demoAppNameToken is the placeholder the demo template uses for the project
// name (replaced verbatim throughout, no Go text/template — the Vue files are
// full of `{{ }}` that must survive untouched).
const demoAppNameToken = "__GOLEO_APP_NAME__"

// demoVersionToken is the placeholder for the goleo version the generated go.mod
// requires. It is filled from the CLI's own version (scaffoldGoleoVersion) rather
// than being passed in, because it is a property of this binary, not of the
// caller's project.
const demoVersionToken = "__GOLEO_VERSION__"

// extractDemoTemplate writes the full-featured "demo" project (the goleo new
// demo template, embedded under templates/demo) into destDir, substituting the
// project name and restoring on-disk names the embed can't hold as-is: `*.tmpl`
// → real extension (so `go build ./cli/...` never compiles the template's Go
// files), and `gitignore` → `.gitignore` (go:embed skips dotfiles).
func extractDemoTemplate(destDir, appName string) error {
	root := "templates/demo"
	return fs.WalkDir(mobileTemplates, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = strings.TrimSuffix(rel, ".tmpl")
		if filepath.Base(rel) == "gitignore" {
			rel = filepath.Join(filepath.Dir(rel), ".gitignore")
		}
		data, err := mobileTemplates.ReadFile(path)
		if err != nil {
			return err
		}
		content := strings.ReplaceAll(string(data), demoAppNameToken, appName)
		content = strings.ReplaceAll(content, demoVersionToken, scaffoldGoleoVersion())
		target := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, []byte(content), 0644)
	})
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
