package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRawGoleoJSON writes an exact document, unlike frontendconfig_test.go's
// writeGoleoJSON which wraps a fragment in a fixed envelope.
func writeRawGoleoJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "goleo.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The headline fix: goleo.json was parsed in four places, each swallowing parse
// errors and returning defaults, so a trailing comma produced a successfully
// built app carrying the wrong applicationId, bundle identifier and version —
// with no diagnostic at all.
func TestMalformedGoleoJSONIsAnError(t *testing.T) {
	dir := t.TempDir()
	writeRawGoleoJSON(t, dir, `{
	  "app_name": "My App",
	  "version": "2.0.0",
	}`) // trailing comma

	if _, _, err := parseGoleoJSON(dir); err == nil {
		t.Fatal("a trailing comma must be reported, not silently ignored")
	} else if !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("error should name the problem, got %v", err)
	}
}

// A key of the wrong type is also a misbuild waiting to happen: the old
// `raw["version"].(string)` type assertion just failed and fell back to a default.
func TestWrongTypedFieldIsAnError(t *testing.T) {
	dir := t.TempDir()
	writeRawGoleoJSON(t, dir, `{"version": 2.0}`)
	if _, _, err := parseGoleoJSON(dir); err == nil {
		t.Error(`"version": 2.0 (number, not string) should be reported`)
	}
}

// A missing file is not an error — several commands run without one.
func TestMissingGoleoJSONIsNotAnError(t *testing.T) {
	cfg, found, err := parseGoleoJSON(t.TempDir())
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if found {
		t.Error("found should be false")
	}
	if cfg.AppName != "" {
		t.Error("expected a zero config")
	}
}

// Unknown keys must keep working — configs legitimately carry extra keys and
// DisallowUnknownFields would break them for no safety gain.
func TestUnknownKeysAreTolerated(t *testing.T) {
	dir := t.TempDir()
	writeRawGoleoJSON(t, dir, `{"app_name":"X","some_future_key":{"a":1},"$schema":"./x.json"}`)
	cfg, _, err := parseGoleoJSON(dir)
	if err != nil {
		t.Fatalf("unknown keys must be tolerated, got %v", err)
	}
	if cfg.AppName != "X" {
		t.Errorf("AppName = %q, want X", cfg.AppName)
	}
}

// These three keys were documented in the guide but read by no Go code: the
// Android min_sdk was hardcoded in the gradle template, and the iOS bundle id was
// derived from the ANDROID package name, so setting it did nothing.
func TestPreviouslyDeadMobileFieldsAreNowRead(t *testing.T) {
	dir := t.TempDir()
	writeRawGoleoJSON(t, dir, `{
	  "version": "1.4.2",
	  "app_name": "Demo",
	  "mobile": {
	    "android": {"package_name": "com.example.droid", "min_sdk": 26, "target_sdk": 35},
	    "ios": {"bundle_identifier": "com.example.ios", "deployment_target": "16.2"}
	  }
	}`)
	cfg := loadMobileConfig(dir)

	if cfg.MinSDK != 26 {
		t.Errorf("MinSDK = %d, want 26 (mobile.android.min_sdk was dead)", cfg.MinSDK)
	}
	if cfg.TargetSDK != 35 {
		t.Errorf("TargetSDK = %d, want 35", cfg.TargetSDK)
	}
	if cfg.IOSBundleID != "com.example.ios" {
		t.Errorf("IOSBundleID = %q, want com.example.ios (was derived from the Android package)", cfg.IOSBundleID)
	}
	if cfg.IOSDeploymentTarget != "16.2" {
		t.Errorf("IOSDeploymentTarget = %q, want 16.2", cfg.IOSDeploymentTarget)
	}
	if cfg.VersionName != "1.4.2" {
		t.Errorf("VersionName = %q, want 1.4.2", cfg.VersionName)
	}
}

// Without an explicit ios.bundle_identifier the Android package is still the
// fallback, so existing projects keep their current iOS identity.
func TestIOSBundleIDFallsBackToAndroidPackage(t *testing.T) {
	dir := t.TempDir()
	writeRawGoleoJSON(t, dir, `{"mobile":{"android":{"package_name":"com.example.only"}}}`)
	if got := loadMobileConfig(dir).IOSBundleID; got != "com.example.only" {
		t.Errorf("IOSBundleID = %q, want the Android package as fallback", got)
	}
}

// extractPackageName matched the literal `"package_name": "` — exactly one space,
// no nesting awareness — so any reformatting fell through to "com.goleo.app".
// `adb install` then succeeded while `am start` launched a package that wasn't
// there. Every one of these formats must now resolve correctly.
func TestPackageNameSurvivesReformatting(t *testing.T) {
	formats := map[string]string{
		"canonical":     `{"mobile": {"android": {"package_name": "com.example.x"}}}`,
		"compact":       `{"mobile":{"android":{"package_name":"com.example.x"}}}`,
		"extra spaces":  `{"mobile": {"android": {"package_name"   :    "com.example.x"}}}`,
		"line break":    "{\"mobile\":{\"android\":{\"package_name\":\n\"com.example.x\"}}}",
		"tabs":          "{\n\t\"mobile\": {\n\t\t\"android\": {\n\t\t\t\"package_name\": \"com.example.x\"\n\t\t}\n\t}\n}",
		"key elsewhere": `{"bundle":{"description":"has package_name: \"decoy\" inside"},"mobile":{"android":{"package_name":"com.example.x"}}}`,
	}
	for name, doc := range formats {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeRawGoleoJSON(t, dir, doc)
			if got := loadMobileConfig(dir).PackageName; got != "com.example.x" {
				t.Errorf("PackageName = %q, want com.example.x", got)
			}
		})
	}
}

func TestVersionCodeFromSemver(t *testing.T) {
	ok := map[string]int{
		"1.0.0":     10000,
		"0.8.9":     809,
		"1.2.3":     10203,
		"2.10.5":    21005, // 2*10000 + 10*100 + 5
		"1.4.2-rc1": 10402,
		"v1.4.2":    10402,
		"1.4":       10400,
		"3":         30000,
	}
	for in, want := range ok {
		got, valid := versionCodeFromSemver(in)
		if !valid {
			t.Errorf("versionCodeFromSemver(%q) reported invalid", in)
			continue
		}
		if got != want {
			t.Errorf("versionCodeFromSemver(%q) = %d, want %d", in, got, want)
		}
	}
	// Must be monotonic across releases, which is what Play requires.
	prev := 0
	for _, v := range []string{"0.8.8", "0.8.9", "0.9.0", "1.0.0", "1.0.1"} {
		got, _ := versionCodeFromSemver(v)
		if got <= prev {
			t.Errorf("versionCode for %s (%d) did not increase over %d", v, got, prev)
		}
		prev = got
	}
	for _, bad := range []string{"", "abc", "1.2.3.4", "1.100.0", "1.2.100", "-1.0.0"} {
		if _, valid := versionCodeFromSemver(bad); valid {
			t.Errorf("versionCodeFromSemver(%q) should be invalid", bad)
		}
	}
}

// An explicit version_code always wins, since a derived one can't express a
// re-upload of the same semver.
func TestExplicitVersionCodeOverridesDerived(t *testing.T) {
	dir := t.TempDir()
	writeRawGoleoJSON(t, dir, `{"version":"1.2.3","mobile":{"android":{"version_code":9999}}}`)
	if got := loadMobileConfig(dir).VersionCode; got != 9999 {
		t.Errorf("VersionCode = %d, want the explicit 9999", got)
	}
}

func TestValidateGoleoJSONCatchesBadIdentifiers(t *testing.T) {
	bad := []goleoJSON{
		{Mobile: mobileSection{Android: androidSection{PackageName: "notreversedns"}}},
		{Mobile: mobileSection{IOS: iosSection{BundleIdentifier: "nodots"}}},
		{Mobile: mobileSection{Android: androidSection{MinSDK: 5}}},
	}
	for i, cfg := range bad {
		if err := validateGoleoJSON(cfg); err == nil {
			t.Errorf("case %d should be rejected", i)
		}
	}
	good := goleoJSON{Mobile: mobileSection{
		Android: androidSection{PackageName: "com.example.app", MinSDK: 24},
		IOS:     iosSection{BundleIdentifier: "com.example.app"},
	}}
	if err := validateGoleoJSON(good); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

// The bundle and frontend adapters must behave exactly as before for existing
// configs — this is a refactor, not a behaviour change, for anything well-formed.
func TestAdaptersPreserveExistingBehaviour(t *testing.T) {
	dir := t.TempDir()
	writeRawGoleoJSON(t, dir, `{
	  "version": "3.1.4",
	  "app_name": "Bundled",
	  "frontend": {"directory":"web","dev_command":"npm run dev","dev_port":4000,"build_command":"npm run build","dist_dir":".output/public"},
	  "bundle": {"identifier":"com.example.bundled","publisher":"Acme","category":"Utility","icon":"icon.png","url_scheme":"myapp"}
	}`)

	b := loadBundleConfig(dir)
	if b.AppName != "Bundled" || b.Version != "3.1.4" || b.Identifier != "com.example.bundled" {
		t.Errorf("bundleConfig mismatch: %+v", b)
	}
	if b.Publisher != "Acme" || b.Category != "Utility" || b.Icon != "icon.png" || b.URLScheme != "myapp" {
		t.Errorf("bundleConfig bundle-section mismatch: %+v", b)
	}

	f := loadFrontendConfig(dir)
	if f.Directory != "web" || f.DevCommand != "npm run dev" || f.DevPort != 4000 ||
		f.BuildCommand != "npm run build" || f.DistDir != ".output/public" {
		t.Errorf("frontendConfig mismatch: %+v", f)
	}
}

// Defaults must survive a config that sets nothing, so a bare project builds.
func TestAdapterDefaults(t *testing.T) {
	dir := t.TempDir()
	writeRawGoleoJSON(t, dir, `{}`)
	b := loadBundleConfig(dir)
	if b.AppName != "Goleo App" || b.Version != "0.1.0" || b.Identifier != "com.goleo.app" {
		t.Errorf("bundle defaults lost: %+v", b)
	}
	m := loadMobileConfig(dir)
	if m.PackageName != "com.goleo.app" || m.AppName != "Goleo App" || m.DevPort != 5173 {
		t.Errorf("mobile defaults lost: %+v", m)
	}
	if m.MinSDK != defaultAndroidMinSDK || m.TargetSDK != defaultAndroidTargetSDK {
		t.Errorf("sdk defaults lost: %+v", m)
	}
}
