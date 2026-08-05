package cmd

import (
	"encoding/xml"
	"regexp"
	"strings"
	"testing"
)

func msixTestBundle() bundleConfig {
	return bundleConfig{AppName: "My App", Version: "1.4.2", Description: "Does things"}
}

func msixTestRaw() msixSection {
	return msixSection{IdentityName: "12345Acme.MyApp", Publisher: "CN=Acme Ltd, O=Acme Ltd, C=GB"}
}

func msixTestConfig() msixConfig {
	return msixConfig{
		IdentityName: "A.B", Publisher: "CN=X", Version: "1.0.0.0", Arch: "x64",
		DisplayName: "App", Description: "d",
	}
}

// EVERY asset the manifest references must be one the packager actually generates.
//
// This is the bug that made the first real `makeappx pack` fail: the manifest named six
// logo files while the code had skipped generating them, so makeappx refused the package
// with "The file name ... doesn't exist in the package" — once per asset. A manifest
// referencing a file the layout does not contain is not a package.
func TestManifestOnlyReferencesGeneratedAssets(t *testing.T) {
	manifest := appxManifest(msixTestConfig(), "app.exe")

	re := regexp.MustCompile(`Assets\\([A-Za-z0-9._\-]+\.png)`)
	found := re.FindAllStringSubmatch(manifest, -1)
	if len(found) == 0 {
		t.Fatal("the manifest references no assets at all — it needs at least a StoreLogo")
	}
	for _, m := range found {
		if _, ok := msixAssets[m[1]]; !ok {
			t.Errorf("manifest references Assets\\%s, which msixAssets does not generate — "+
				"makeappx will refuse the package", m[1])
		}
	}

	for _, required := range []string{"StoreLogo.png", "Square150x150Logo.png", "Square44x44Logo.png"} {
		if _, ok := msixAssets[required]; !ok {
			t.Errorf("msixAssets is missing the required %s", required)
		}
	}
}

// The manifest goes to makeappx, which validates it as XML. Its values come from
// goleo.json, so an ampersand in a company name or an angle bracket in a description must
// not produce something invalid. The first version used %q — Go quoting, not XML escaping
// — and makeappx rejected the manifest outright.
func TestManifestIsValidXMLWithHostileMetadata(t *testing.T) {
	cfg := msixConfig{
		IdentityName:         "12345Acme.MyApp",
		Publisher:            "CN=Smith & Sons, O=" + `"Quoted"` + " Ltd, C=GB",
		PublisherDisplayName: "Smith & Sons",
		DisplayName:          "App <beta> & " + `"friends"`,
		Description:          "A <script>alert(1)</script> & more",
		Version:              "1.0.0.0",
		Arch:                 "x64",
	}
	manifest := appxManifest(cfg, "app & thing.exe")

	var probe any
	if err := xml.Unmarshal([]byte(manifest), &probe); err != nil {
		t.Fatalf("manifest is not valid XML: %v\n%s", err, manifest)
	}
	// Raw metacharacters must not survive into the document.
	for _, bad := range []string{"<script>", "<beta>", `O="Quoted"`} {
		if strings.Contains(manifest, bad) {
			t.Errorf("unescaped %q leaked into the manifest:\n%s", bad, manifest)
		}
	}
	// And the escaped forms must be present, i.e. the values were not simply dropped.
	for _, want := range []string{"&amp;", "&lt;", "&quot;"} {
		if !strings.Contains(manifest, want) {
			t.Errorf("expected %s in the escaped manifest:\n%s", want, manifest)
		}
	}
}

// A goleo app is a Win32 executable, so the package must declare itself full-trust.
// Without these it would be treated as a UWP app and the Go backend could not run.
func TestManifestDeclaresFullTrustWin32(t *testing.T) {
	manifest := appxManifest(msixTestConfig(), "app.exe")
	for _, want := range []string{
		`EntryPoint="Windows.FullTrustApplication"`,
		`<rescap:Capability Name="runFullTrust" />`,
		`Executable="app.exe"`,
	} {
		if !strings.Contains(manifest, want) {
			t.Errorf("manifest is missing %s — a packaged Win32 app needs it:\n%s", want, manifest)
		}
	}
}

// The Store reserves the fourth version part and rejects a non-zero revision.
func TestMSIXVersionAlwaysHasRevisionZero(t *testing.T) {
	ok := map[string]string{
		"1.2.3":     "1.2.3.0",
		"v1.2.3":    "1.2.3.0",
		"1.2":       "1.2.0.0",
		"1":         "1.0.0.0",
		"1.2.3.0":   "1.2.3.0",
		"1.2.3-rc1": "1.2.3.0", // prerelease dropped: MSIX versions are numeric only
		"0.1.0":     "0.1.0.0",
	}
	for in, want := range ok {
		got, err := msixVersion(in)
		if err != nil {
			t.Errorf("msixVersion(%q) errored: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("msixVersion(%q) = %q, want %q", in, got, want)
		}
	}

	err := func() error { _, e := msixVersion("1.2.3.4"); return e }()
	if err == nil {
		t.Fatal("a non-zero fourth part should be refused — the Store rejects it")
	}
	if !strings.Contains(err.Error(), "1.2.3") {
		t.Errorf("the error should suggest the corrected version:\n%v", err)
	}

	for _, bad := range []string{"", "abc", "1.x.3", "1.2.3.4.5", "70000.0.0", "-1.0.0"} {
		if _, err := msixVersion(bad); err == nil {
			t.Errorf("msixVersion(%q) should have failed", bad)
		}
	}
}

// Identity is the one thing goleo must not guess: a wrong Name or Publisher builds and
// signs happily, then fails at install or submission with nothing pointing at the cause.
func TestMSIXIdentityIsRequiredAndValidated(t *testing.T) {
	b := msixTestBundle()

	if _, err := resolveMSIXConfig(b, msixSection{}, "amd64"); err == nil {
		t.Error("a missing identity_name should be refused")
	} else if !strings.Contains(err.Error(), "Partner Center") {
		t.Errorf("the error should point at Partner Center:\n%v", err)
	}

	// Note "noDots" is NOT here: a single-segment name is legal for MSIX. That is a real
	// difference from an Android package name, which must have two segments because it
	// becomes a Java package — assuming the same rule here would reject a legitimate
	// Partner Center name.
	for _, bad := range []string{"ab", "has space.App", "-leading.App", "a..b", "under_score.App", strings.Repeat("x", 51) + ".A"} {
		if _, err := resolveMSIXConfig(b, msixSection{IdentityName: bad, Publisher: "CN=X"}, "amd64"); err == nil {
			t.Errorf("identity_name %q should be refused", bad)
		}
	}

	// A single segment is legal and must be accepted.
	if _, err := resolveMSIXConfig(b, msixSection{IdentityName: "MyApp", Publisher: "CN=X"}, "amd64"); err != nil {
		t.Errorf("a single-segment identity_name is legal for MSIX: %v", err)
	}

	if _, err := resolveMSIXConfig(b, msixSection{IdentityName: "A.B"}, "amd64"); err == nil {
		t.Error("a missing publisher should be refused")
	}
	// The common mistake: a bare company name instead of the certificate subject.
	_, err := resolveMSIXConfig(b, msixSection{IdentityName: "A.B", Publisher: "Acme Ltd"}, "amd64")
	if err == nil {
		t.Error("a publisher without CN= should be refused — it must be the certificate subject")
	} else if !strings.Contains(err.Error(), "CN=") {
		t.Errorf("the error should say it needs CN=:\n%v", err)
	}
}

// publisher_display_name is only a display string, so it falls back rather than erroring.
func TestPublisherDisplayNameFallsBack(t *testing.T) {
	b := msixTestBundle()
	b.Publisher = "Acme Limited"
	cfg, err := resolveMSIXConfig(b, msixTestRaw(), "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublisherDisplayName != "Acme Limited" {
		t.Errorf("should fall back to bundle.publisher, got %q", cfg.PublisherDisplayName)
	}

	b.Publisher = ""
	cfg, err = resolveMSIXConfig(b, msixTestRaw(), "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublisherDisplayName != "Acme Ltd" {
		t.Errorf("should fall back to the certificate subject's CN, got %q", cfg.PublisherDisplayName)
	}
}

func TestMSIXArchMapping(t *testing.T) {
	for goarch, want := range map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"} {
		got, err := msixArchFor(goarch)
		if err != nil || got != want {
			t.Errorf("msixArchFor(%q) = %q, %v; want %q", goarch, got, err, want)
		}
	}
	if _, err := msixArchFor("riscv64"); err == nil {
		t.Error("an unsupported arch should be refused rather than guessed")
	}
}

// NSIS stays the default: an MSIX needs Partner Center identity, so defaulting to it would
// turn every existing project's --bundle into a config error.
func TestWindowsFormatDefaultsToNSIS(t *testing.T) {
	for in, want := range map[string]windowsBundleFormat{
		"":     windowsFormatNSIS,
		"nsis": windowsFormatNSIS,
		"msix": windowsFormatMSIX,
		"MSIX": windowsFormatMSIX,
		"both": windowsFormatBoth,
	} {
		got, err := resolveWindowsFormat(in)
		if err != nil || got != want {
			t.Errorf("resolveWindowsFormat(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := resolveWindowsFormat("appx"); err == nil {
		t.Error("an unknown format should be refused")
	}
}
