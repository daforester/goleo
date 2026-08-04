package cmd

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MSIX packaging for the Microsoft Store (and for sideloading).
//
// An MSIX is a signed zip with an AppxManifest.xml and a fixed set of logo assets.
// `makeappx pack` builds it and the existing signtool path signs it, so the work here is
// almost entirely in getting the manifest and identity right — which is where MSIX is
// unforgiving:
//
//   - Identity/Name must match the name reserved in Partner Center EXACTLY.
//   - Identity/Publisher must match the signing certificate's subject exactly, character
//     for character. A mismatch fails at install with a generic error that does not say
//     which of the two is wrong, so goleo checks the shape up front and says what it saw.
//   - Version must be four parts, and the Store REJECTS a package whose fourth part is
//     non-zero: the revision field is reserved for the Store itself.
//
// A goleo app is a plain Win32 executable, so the package declares
// EntryPoint="Windows.FullTrustApplication" and the restricted `runFullTrust` capability.
// That is the supported way to ship a desktop app through the Store, and it is why the
// loopback bridge keeps working — a full-trust package is not sandboxed the way a UWP app
// is. (Mac App Store is the opposite case, and is why that remains gated behind a spike.)

// msixConfig is the resolved identity and metadata for a package.
type msixConfig struct {
	IdentityName         string // Partner Center reserved name, e.g. "12345Contoso.MyApp"
	Publisher            string // certificate subject, e.g. "CN=Contoso, O=Contoso, C=GB"
	PublisherDisplayName string // shown in the Store listing
	DisplayName          string
	Description          string
	Version              string // four-part, revision 0
	Arch                 string // x64 | arm64 | x86
}

// msixArchFor maps a GOARCH to the ProcessorArchitecture MSIX expects.
func msixArchFor(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "arm64", nil
	case "386":
		return "x86", nil
	default:
		return "", fmt.Errorf("MSIX does not support GOARCH %q (want amd64, arm64 or 386)", goarch)
	}
}

// msixVersion returns a four-part version with revision 0.
//
// The Store rejects a package whose fourth part is non-zero — it reserves the revision
// field — and it also rejects a three-part version outright. to4PartVersion (used for the
// exe's VERSIONINFO) is not reusable here because it does not guarantee that trailing 0.
func msixVersion(v string) (string, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(trimmed, "-+"); i >= 0 {
		trimmed = trimmed[:i] // drop -rc1 / +build; MSIX versions are numeric only
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) > 4 {
		return "", fmt.Errorf("version %q has more than four parts", v)
	}
	nums := make([]int, 4)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 65535 {
			return "", fmt.Errorf("version %q: part %q must be a number from 0 to 65535", v, p)
		}
		nums[i] = n
	}
	if len(parts) == 4 && nums[3] != 0 {
		return "", fmt.Errorf("version %q ends in .%d — the Store reserves the fourth part "+
			"and rejects a non-zero revision; use %d.%d.%d instead",
			v, nums[3], nums[0], nums[1], nums[2])
	}
	return fmt.Sprintf("%d.%d.%d.0", nums[0], nums[1], nums[2]), nil
}

// msixIdentityNameRe is the Store's rule for a package Name: dot-separated alphanumeric
// segments. Partner Center hands you one; typing it by hand is where errors happen.
var msixIdentityNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9\-]*(\.[A-Za-z0-9][A-Za-z0-9\-]*)*$`)

// resolveMSIXConfig builds the identity from goleo.json, failing with actionable messages.
//
// It refuses to invent an identity. A wrong Name or Publisher produces a package that
// builds and signs happily and then fails to install, or is rejected at submission — so
// guessing here would trade a clear error now for a confusing one later.
func resolveMSIXConfig(bundle bundleConfig, raw msixSection, goarch string) (msixConfig, error) {
	var cfg msixConfig

	name := strings.TrimSpace(raw.IdentityName)
	if name == "" {
		return cfg, fmt.Errorf("MSIX needs windows.msix.identity_name in goleo.json.\n" +
			"  This is the Package/Identity Name reserved in Partner Center (Product\n" +
			"  identity → Package Name), e.g. \"12345Contoso.MyApp\". It must match EXACTLY:\n" +
			"  a package whose name differs is rejected at submission.")
	}
	// 3-50 characters is the Store's actual rule. A single segment is allowed, unlike an
	// Android package name — that one must have two because it becomes a Java package,
	// and conflating the two rules would reject a legitimate reserved name.
	if len(name) < 3 || len(name) > 50 || !msixIdentityNameRe.MatchString(name) {
		return cfg, fmt.Errorf("windows.msix.identity_name %q is not a valid package name "+
			"(3-50 chars; letters, digits, dots and dashes; each dot-separated segment must "+
			"start with a letter or digit)", name)
	}
	cfg.IdentityName = name

	pub := strings.TrimSpace(raw.Publisher)
	if pub == "" {
		return cfg, fmt.Errorf("MSIX needs windows.msix.publisher in goleo.json.\n" +
			"  This is the SUBJECT OF YOUR SIGNING CERTIFICATE, not a company name — e.g.\n" +
			"  \"CN=Contoso Ltd, O=Contoso Ltd, L=London, C=GB\". Partner Center shows the\n" +
			"  exact string under Product identity → Publisher. If it does not match the\n" +
			"  certificate character for character, the package installs nowhere and the\n" +
			"  error does not say which side is wrong.")
	}
	// A bare company name is the mistake people make here, and it is worth catching:
	// the resulting package fails at install with nothing pointing at this field.
	if !strings.Contains(pub, "=") {
		return cfg, fmt.Errorf("windows.msix.publisher %q does not look like a certificate "+
			"subject — it needs at least CN=..., e.g. \"CN=Contoso Ltd, O=Contoso Ltd, C=GB\"", pub)
	}
	cfg.Publisher = pub

	cfg.PublisherDisplayName = strings.TrimSpace(raw.PublisherDisplayName)
	if cfg.PublisherDisplayName == "" {
		// Falls back to bundle.publisher, then the CN of the certificate subject, because
		// this one is only a display string and a sensible default beats an error.
		cfg.PublisherDisplayName = strings.TrimSpace(bundle.Publisher)
	}
	if cfg.PublisherDisplayName == "" {
		cfg.PublisherDisplayName = cnOf(pub)
	}

	cfg.DisplayName = bundle.AppName
	cfg.Description = bundle.Description
	if cfg.Description == "" {
		cfg.Description = bundle.AppName
	}

	ver, err := msixVersion(bundle.Version)
	if err != nil {
		return cfg, fmt.Errorf("MSIX version: %w", err)
	}
	cfg.Version = ver

	arch, err := msixArchFor(goarch)
	if err != nil {
		return cfg, err
	}
	cfg.Arch = arch
	return cfg, nil
}

// cnOf extracts the CN from a certificate subject, for a display-name fallback.
func cnOf(subject string) string {
	for _, part := range strings.Split(subject, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToUpper(part), "CN=") {
			return strings.TrimSpace(part[3:])
		}
	}
	return subject
}

// msixAssets are the logo files a package must ship.
//
// The Store requires the three named below at minimum; without them makeappx succeeds and
// the app appears with a blank tile. They are generated from the same single bundle.icon
// PNG as every other platform's icons, so there is nothing extra to supply.
var msixAssets = map[string]int{
	"Square44x44Logo.png":   44,  // taskbar / app list
	"Square150x150Logo.png": 150, // medium Start tile
	"StoreLogo.png":         50,  // Store listing and installer
	// Target-size variants Windows picks for specific surfaces. Cheap to include and
	// their absence is a visibly worse result rather than an error.
	"Square44x44Logo.targetsize-24_altform-unplated.png": 24,
	"Square310x310Logo.png":                              310,
	"Wide310x150Logo.png":                                150,
}

// appxManifest renders AppxManifest.xml.
//
// Written out rather than templated from a file because it has to interleave two
// namespaces and the escaping matters: DisplayName and Description are user strings that
// land in XML text nodes.
func appxManifest(cfg msixConfig, exeName string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<Package
  xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"
  xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10"
  xmlns:rescap="http://schemas.microsoft.com/appx/manifest/foundation/windows10/restrictedcapabilities"
  IgnorableNamespaces="uap rescap">
` + "\n")
	fmt.Fprintf(&b, "  <Identity Name=%s Publisher=%s Version=%s ProcessorArchitecture=%s />\n",
		xmlAttr(cfg.IdentityName), xmlAttr(cfg.Publisher), xmlAttr(cfg.Version), xmlAttr(cfg.Arch))

	b.WriteString("\n  <Properties>\n")
	fmt.Fprintf(&b, "    <DisplayName>%s</DisplayName>\n", xmlEscape(cfg.DisplayName))
	fmt.Fprintf(&b, "    <PublisherDisplayName>%s</PublisherDisplayName>\n", xmlEscape(cfg.PublisherDisplayName))
	b.WriteString("    <Logo>Assets\\StoreLogo.png</Logo>\n")
	fmt.Fprintf(&b, "    <Description>%s</Description>\n", xmlEscape(cfg.Description))
	b.WriteString("  </Properties>\n")

	// Windows 10 1809 is the floor for a full-trust packaged desktop app.
	b.WriteString(`
  <Dependencies>
    <TargetDeviceFamily Name="Windows.Desktop" MinVersion="10.0.17763.0" MaxVersionTested="10.0.22621.0" />
  </Dependencies>

  <Resources>
    <Resource Language="en-us" />
  </Resources>
`)

	b.WriteString("\n  <Applications>\n")
	// EntryPoint Windows.FullTrustApplication is what makes this a packaged WIN32 app
	// rather than a UWP one — it is why the Go backend and its loopback server work.
	fmt.Fprintf(&b, "    <Application Id=\"App\" Executable=%s EntryPoint=\"Windows.FullTrustApplication\">\n",
		xmlAttr(exeName))
	fmt.Fprintf(&b, "      <uap:VisualElements DisplayName=%s Description=%s\n",
		xmlAttr(cfg.DisplayName), xmlAttr(cfg.Description))
	b.WriteString("        BackgroundColor=\"transparent\"\n")
	b.WriteString("        Square150x150Logo=\"Assets\\Square150x150Logo.png\"\n")
	b.WriteString("        Square44x44Logo=\"Assets\\Square44x44Logo.png\">\n")
	b.WriteString("        <uap:DefaultTile Wide310x150Logo=\"Assets\\Wide310x150Logo.png\" Square310x310Logo=\"Assets\\Square310x310Logo.png\" />\n")
	b.WriteString("      </uap:VisualElements>\n")
	b.WriteString("    </Application>\n")
	b.WriteString("  </Applications>\n")

	// runFullTrust is a RESTRICTED capability: Partner Center requires a short
	// justification for it on submission. It is unavoidable for a packaged Win32 app.
	b.WriteString(`
  <Capabilities>
    <rescap:Capability Name="runFullTrust" />
  </Capabilities>
</Package>
`)
	return b.String()
}

// xmlEscape escapes text destined for an XML text node. DisplayName and Description come
// from goleo.json, so an ampersand in a company name must not produce invalid XML.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// sortedAssetNames gives the asset list a stable order, so a generated layout and any
// test over it do not depend on map iteration.
func sortedAssetNames() []string {
	names := make([]string, 0, len(msixAssets))
	for n := range msixAssets {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// findMakeAppx locates makeappx.exe from the Windows SDK.
//
// PATH first, then the newest SDK under the standard Windows Kits location — the SDK does
// not put its tools on PATH, so requiring that would make this fail for almost everyone
// who has the SDK installed.
func findMakeAppx() (string, error) {
	if p, err := exec.LookPath("makeappx.exe"); err == nil {
		return p, nil
	}
	var candidates []string
	for _, root := range []string{
		`C:\Program Files (x86)\Windows Kits\10\bin`,
		`C:\Program Files\Windows Kits\10\bin`,
	} {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		var versions []string
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "10.") {
				versions = append(versions, e.Name())
			}
		}
		// Newest SDK last, so prefer it.
		sort.Strings(versions)
		for i := len(versions) - 1; i >= 0; i-- {
			for _, arch := range []string{"x64", "x86", "arm64"} {
				candidates = append(candidates, filepath.Join(root, versions[i], arch, "makeappx.exe"))
			}
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("makeappx.exe not found — it ships with the Windows SDK and is not\n" +
		"  placed on PATH. Install the Windows 10/11 SDK (winget install Microsoft.WindowsSDK,\n" +
		"  or the \"Windows 11 SDK\" component in the Visual Studio Installer) and retry.")
}

// bundleMSIX packages the built binary into a signed .msix.
//
// Layout staged on disk, then `makeappx pack`, then the existing signtool path — the same
// signing code the NSIS installer uses, so a certificate configured for one works for the
// other. An unsigned MSIX cannot be installed at all (Windows refuses it outright), so
// this warns loudly rather than quietly producing something unusable.
func bundleMSIX(binaryPath string, bundle bundleConfig, raw msixSection, outDir, goarch string, sc signConfig) error {
	cfg, err := resolveMSIXConfig(bundle, raw, goarch)
	if err != nil {
		return err
	}
	tool, err := findMakeAppx()
	if err != nil {
		return err
	}

	exeName := filepath.Base(binaryPath)
	stage := filepath.Join(outDir, "msix-stage")
	if err := os.RemoveAll(stage); err != nil {
		return fmt.Errorf("msix: clearing the staging dir: %w", err)
	}
	assetsDir := filepath.Join(stage, "Assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return fmt.Errorf("msix: creating the staging dir: %w", err)
	}

	if err := copyFile(binaryPath, filepath.Join(stage, exeName)); err != nil {
		return fmt.Errorf("msix: staging the executable: %w", err)
	}

	// Logos come from the same single bundle.icon PNG as every other platform's icons.
	// Without them makeappx still succeeds and the app shows a blank tile, so an absent
	// icon is a warning rather than a failure — but say so, because a blank tile in a
	// Store listing is not something to discover after submitting.
	// resolveSourceIcon + loadPNG rather than mobileIconSource(), which re-reads
	// goleo.json from "." — the bundle config is already resolved here.
	// An icon is REQUIRED, not optional. The manifest references the logo files, so
	// makeappx hard-fails without them ("The file name ... doesn't exist in the package")
	// — warning and carrying on just precedes an obscure failure. The Store requires a
	// logo anyway, and generating a placeholder would risk shipping it.
	src, iconOK := msixIconSource(bundle)
	if !iconOK {
		return fmt.Errorf("MSIX needs bundle.icon in goleo.json — a square PNG, 1024x1024\n" +
			"  recommended. Every logo in the package is generated from it, the manifest\n" +
			"  references them, and the Store requires a logo, so there is no useful\n" +
			"  package without one.")
	}
	for _, name := range sortedAssetNames() {
		if err := writeResizedPNG(src, msixAssets[name], filepath.Join(assetsDir, name)); err != nil {
			return fmt.Errorf("msix: generating %s: %w", name, err)
		}
	}
	fmt.Printf("  Generated %d logo assets from bundle.icon\n", len(msixAssets))

	manifestPath := filepath.Join(stage, "AppxManifest.xml")
	if err := os.WriteFile(manifestPath, []byte(appxManifest(cfg, exeName)), 0o644); err != nil {
		return fmt.Errorf("msix: writing AppxManifest.xml: %w", err)
	}

	outFile, err := filepath.Abs(filepath.Join(outDir, installerName(bundle, ".msix", "")))
	if err != nil {
		return err
	}
	_ = os.Remove(outFile) // makeappx refuses to overwrite

	fmt.Printf("  Packing %s (%s, v%s)...\n", filepath.Base(outFile), cfg.Arch, cfg.Version)
	pack := exec.Command(tool, "pack", "/d", stage, "/p", outFile, "/o")
	pack.Stdout, pack.Stderr = os.Stdout, os.Stderr
	if err := pack.Run(); err != nil {
		return fmt.Errorf("msix: makeappx pack failed: %w", err)
	}
	if _, err := os.Stat(outFile); err != nil {
		return fmt.Errorf("msix: makeappx reported success but produced no package at %s: %w", outFile, err)
	}

	// Windows refuses to install an unsigned MSIX, and the Publisher in the manifest must
	// match the signing certificate's subject exactly — so an unsigned package is not a
	// usable artifact, unlike an unsigned .exe which merely warns.
	if buildNoSign {
		fmt.Println("  --no-sign: the .msix is UNSIGNED and Windows will refuse to install it")
	} else if !sc.windowsEnabled() {
		fmt.Println("  Warning: no signing certificate configured, so the .msix is unsigned and")
		fmt.Println("    Windows will refuse to install it. Set GOLEO_WIN_CERT +")
		fmt.Println("    GOLEO_WIN_CERT_PASSWORD, and make sure the certificate subject matches")
		fmt.Printf("    windows.msix.publisher exactly (%q).\n", cfg.Publisher)
	}
	if err := signWindows(sc, outFile); err != nil {
		return err
	}

	fmt.Printf("  Created %s\n", outFile)
	fmt.Printf("  Identity: %s / %s\n", cfg.IdentityName, cfg.Publisher)
	return nil
}

// msixIconSource loads the single source icon for logo generation.
func msixIconSource(bundle bundleConfig) (image.Image, bool) {
	path, ok := resolveSourceIcon(bundle)
	if !ok {
		return nil, false
	}
	img, err := loadPNG(path)
	if err != nil {
		return nil, false
	}
	return img, true
}

// xmlAttr returns a value ready to use as a quoted XML ATTRIBUTE, escaping included.
//
// Not %q: Go quoting is not XML escaping. The first version used %q for DisplayName,
// Description, Publisher and the executable name, and makeappx rejected the manifest
// outright — "App manifest validation error: An attribute value must not contain '<'" —
// for a description containing an angle bracket. A publisher of "CN=Smith & Sons" would
// have failed the same way, and that is an ordinary company name rather than a contrived
// payload.
func xmlAttr(s string) string {
	return `"` + strings.ReplaceAll(xmlEscape(s), `"`, "&quot;") + `"`
}
