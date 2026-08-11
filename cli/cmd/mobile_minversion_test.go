package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"text/template"
)

func TestIOSMinVersionPrefersTheFlagThenConfig(t *testing.T) {
	cases := []struct {
		flag, config, want string
	}{
		{"", "", defaultIOSDeployTarget},   // neither set
		{"", "16.2", "16.2"},               // config wins over the built-in default
		{"17.0", "16.2", "17.0"},           // the flag is the explicit override
		{"  17.0  ", "16.2", "17.0"},       // and is trimmed
		{"", "  ", defaultIOSDeployTarget}, // blank config is not a value
	}
	for _, c := range cases {
		got, err := resolveIOSMinVersion(c.flag, c.config)
		if err != nil {
			t.Errorf("resolveIOSMinVersion(%q, %q) errored: %v", c.flag, c.config, err)
			continue
		}
		if got != c.want {
			t.Errorf("resolveIOSMinVersion(%q, %q) = %q, want %q", c.flag, c.config, got, c.want)
		}
	}

	// A bad value must be refused here, naming where it came from — gomobile passes it to
	// clang's -miphoneos-version-min, where the error mentions neither goleo nor goleo.json.
	if _, err := resolveIOSMinVersion("", "abc"); err == nil {
		t.Error("a malformed deployment_target should be refused")
	} else if !strings.Contains(err.Error(), "deployment_target") {
		t.Errorf("the error should name the config key it came from:\n%v", err)
	}
	if _, err := resolveIOSMinVersion("nope", "16.0"); err == nil {
		t.Error("a malformed --ios-target should be refused")
	} else if !strings.Contains(err.Error(), "--ios-target") {
		t.Errorf("the error should name the flag it came from:\n%v", err)
	}
	// Note "34" is NOT here. It looks like an Android API level pasted into an iOS field,
	// but since Apple renumbered to iOS 26 a major in the thirties is plausible, so the
	// range cannot catch that mistake without rejecting a legitimate future version.
	// "9.0" and "12" used to be accepted. The shell's Info.plist now declares a
	// UIApplicationSceneManifest and SceneDelegate builds the window, so anything below
	// iOS 13 builds and signs cleanly and then launches to a black screen — refusing it
	// here is the only place that can say so.
	for _, bad := range []string{"0", "1.2.3.4", "15.x", "-1", "8", "9.0", "12", "100"} {
		if _, err := resolveIOSMinVersion(bad, ""); err == nil {
			t.Errorf("--ios-target %q should be refused", bad)
		}
	}
	if _, err := resolveIOSMinVersion("12.0", ""); err == nil {
		t.Error("iOS 12 should be refused")
	} else if !strings.Contains(err.Error(), "UIScene") {
		t.Errorf("the error should say why 13 is the floor, not just that 12 is invalid:\n%v", err)
	}
	for _, ok := range []string{"15", "15.0", "15.4.1", "13.0", "26"} {
		if _, err := resolveIOSMinVersion(ok, ""); err != nil {
			t.Errorf("--ios-target %q should be accepted: %v", ok, err)
		}
	}
}

func TestAndroidMinAPIPrefersTheFlagThenConfig(t *testing.T) {
	cases := []struct {
		flag, config, want int
	}{
		{0, 0, defaultAndroidMinSDK},
		{0, 26, 26},
		{29, 26, 29},
	}
	for _, c := range cases {
		got, err := resolveAndroidMinAPI(c.flag, c.config)
		if err != nil {
			t.Errorf("resolveAndroidMinAPI(%d, %d) errored: %v", c.flag, c.config, err)
			continue
		}
		if got != c.want {
			t.Errorf("resolveAndroidMinAPI(%d, %d) = %d, want %d", c.flag, c.config, got, c.want)
		}
	}
	for _, bad := range []int{1, 20, 100, -5} {
		if _, err := resolveAndroidMinAPI(bad, 0); err == nil {
			t.Errorf("--android-api %d should be refused", bad)
		}
	}
	if _, err := resolveAndroidMinAPI(0, 20); err == nil {
		t.Error("min_sdk 20 should be refused")
	} else if !strings.Contains(err.Error(), "min_sdk") {
		t.Errorf("the error should name the config key:\n%v", err)
	}
}

// The invariant that was actually broken: gomobile builds the Go library against a
// minimum, and the native project declares its own. If the library's minimum is HIGHER
// than the app's, the link fails — "building for iOS 13.0 but goleo.xcframework was built
// for iOS 14.0" — naming a version the user never chose, because it came from a flag
// default competing with the config.
//
// With the defaults as they shipped: --ios-target 14.0 vs deployment_target 15.0 for a
// project with no iOS config at all, and 14.0 vs 13.0 for one that lowers it.
func TestGomobileMinimumNeverExceedsTheProjectDeploymentTarget(t *testing.T) {
	for _, configured := range []string{"", "13.0", "15.0", "16.4", "18.0"} {
		cfg := defaultMobileConfig()
		if configured != "" {
			cfg.IOSDeploymentTarget = configured
		}

		iosMin, err := resolveIOSMinVersion("" /* no flag */, cfg.IOSDeploymentTarget)
		if err != nil {
			t.Fatalf("deployment_target %q: %v", configured, err)
		}

		// What the Xcode project will actually declare, read out of the template rather
		// than restated — that is the value the framework has to be compatible with.
		declared := renderIOSDeploymentTarget(t, cfg)

		if iosMin != declared {
			t.Errorf("deployment_target %q: gomobile builds the framework for iOS %s while "+
				"the Xcode project declares iOS %s; a framework minimum above the app's "+
				"fails to link", configured, iosMin, declared)
		}
	}
}

func TestGomobileAndroidAPINeverExceedsGradleMinSdk(t *testing.T) {
	for _, configured := range []int{0, 21, 24, 26, 30} {
		cfg := defaultMobileConfig()
		if configured != 0 {
			cfg.MinSDK = configured
		}

		minAPI, err := resolveAndroidMinAPI(0 /* no flag */, cfg.MinSDK)
		if err != nil {
			t.Fatalf("min_sdk %d: %v", configured, err)
		}
		if minAPI != cfg.MinSDK {
			t.Errorf("min_sdk %d: gomobile builds the AAR for API %d while Gradle declares "+
				"minSdk %d", configured, minAPI, cfg.MinSDK)
		}
	}
}

// renderIOSDeploymentTarget pulls the iOS version out of the rendered xcodegen.yml, so
// the assertion is against what the build really writes.
func renderIOSDeploymentTarget(t *testing.T, cfg mobileConfig) string {
	t.Helper()
	raw, err := mobileTemplates.ReadFile("templates/ios/xcodegen.yml")
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("x").Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, cfg); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "iOS:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "iOS:")), `"`)
		}
	}
	t.Fatalf("no iOS deployment target in the rendered xcodegen.yml:\n%s", out.String())
	return ""
}

// The scaffolded goleo.json must agree with the compiled-in defaults. While
// mobile.ios.deployment_target was dead config this did not matter, and it had drifted:
// the scaffold wrote "14.0" while a project without the key got 15.0. Now that the key is
// honoured, that drift means `goleo new` silently lowers a new project's minimum iOS
// version relative to the documented default.
func TestScaffoldedConfigMatchesTheCompiledDefaults(t *testing.T) {
	for name, raw := range map[string]string{
		"minimal (templates.go)": tmplGoleoJSON,
		"demo":                   readDemoGoleoJSON(t),
	} {
		var cfg goleoJSON
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Errorf("%s: scaffolded goleo.json is not valid JSON: %v", name, err)
			continue
		}
		if got := cfg.Mobile.IOS.DeploymentTarget; got != "" && got != defaultIOSDeployTarget {
			t.Errorf("%s: scaffolds mobile.ios.deployment_target %q but the compiled default "+
				"is %q — a new project would get a different minimum iOS version than a "+
				"project that omits the key", name, got, defaultIOSDeployTarget)
		}
		if got := cfg.Mobile.Android.MinSDK; got != 0 && got != defaultAndroidMinSDK {
			t.Errorf("%s: scaffolds mobile.android.min_sdk %d but the compiled default is %d",
				name, got, defaultAndroidMinSDK)
		}
	}
}

func readDemoGoleoJSON(t *testing.T) string {
	t.Helper()
	b, err := mobileTemplates.ReadFile("templates/demo/goleo.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The DEV Android project and the RELEASE one must declare the same SDK levels.
//
// android-dev hardcoded compileSdk 36 / minSdk 24 / targetSdk 36 while android used
// mobile.android.{min_sdk,target_sdk}. That is invisible until a project raises min_sdk
// above 24, and then it fails only on the `goleo emulate` path: gomobile builds the AAR
// against the configured minimum and Gradle rejects a library whose minSdk exceeds the
// app's. Making -androidapi follow the config (see resolveAndroidMinAPI) is what turned
// this from a latent inconsistency into a build failure.
func TestDevAndReleaseAndroidProjectsDeclareTheSameSDKLevels(t *testing.T) {
	cfg := defaultMobileConfig()
	cfg.MinSDK = 29
	cfg.TargetSDK = 35
	cfg.PackageName = "com.example.app"
	cfg.VersionName = "2.1.0"

	get := func(path, key string) string {
		t.Helper()
		raw, err := mobileTemplates.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		tmpl, err := template.New("g").Parse(string(raw))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		var out strings.Builder
		if err := tmpl.Execute(&out, cfg); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, line := range strings.Split(out.String(), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "//") {
				continue // the explanatory comments mention the old literals
			}
			if strings.HasPrefix(line, key+" ") || strings.HasPrefix(line, key+"=") {
				_, v, _ := strings.Cut(line, "=")
				return strings.TrimSpace(v)
			}
		}
		t.Fatalf("%s declares no %s:\n%s", path, key, out.String())
		return ""
	}

	const dev = "templates/android-dev/app/build.gradle.kts"
	const rel = "templates/android/app/build.gradle.kts"
	for _, key := range []string{"compileSdk", "minSdk", "targetSdk"} {
		d, r := get(dev, key), get(rel, key)
		if d != r {
			t.Errorf("%s: dev project declares %s, release declares %s — the dev build would "+
				"target a different platform than the one you ship", key, d, r)
		}
		// And it must be the configured value, not a literal that happens to match.
		if key == "minSdk" && d != "29" {
			t.Errorf("minSdk is %s, not the configured 29 — it is hardcoded", d)
		}
		if key == "targetSdk" && d != "35" {
			t.Errorf("targetSdk is %s, not the configured 35 — it is hardcoded", d)
		}
	}

	// The dev build must be identifiable per project. Every goleo dev build was labelled
	// "Goleo (Dev)", so two of them on one device were indistinguishable in the launcher.
	manifest, err := mobileTemplates.ReadFile("templates/android-dev/app/src/main/AndroidManifest.xml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), `android:label="Goleo (Dev)"`) {
		t.Error("the dev manifest hardcodes the launcher label, so every goleo project's dev " +
			"build looks the same on the device")
	}
}
