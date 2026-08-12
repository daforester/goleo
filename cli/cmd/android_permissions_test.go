package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// The manifest used to declare thirteen permissions for EVERY app because the demo
// needed them — CAMERA, RECORD_AUDIO, both LOCATION grants, VIBRATE, NFC and four
// BLUETOOTH_*. Play flags unjustified permissions, and a user installing a note-taking
// app should not be told it wants the camera and their location.
func TestPermissionsFollowEnablementNotEverything(t *testing.T) {
	// An app enabling nothing gets the core set only.
	bare := resolveAndroidPermissions(nil, nil)
	if len(bare.Permissions) != len(corePermissions) {
		t.Errorf("an app enabling no features declared %v, want just the core set", bare.Permissions)
	}
	for _, unwanted := range []string{
		"android.permission.CAMERA",
		"android.permission.ACCESS_FINE_LOCATION",
		"android.permission.NFC",
		"android.permission.BLUETOOTH_SCAN",
		"android.permission.BLUETOOTH_CONNECT",
		"android.permission.VIBRATE",
		"android.permission.BODY_SENSORS",
	} {
		if hasPerm(bare.Permissions, unwanted) {
			t.Errorf("a bare app should not declare %s", unwanted)
		}
	}
	if len(bare.Features) != 0 {
		t.Errorf("a bare app should declare no <uses-feature>, got %v", bare.Features)
	}
	if bare.LegacyXML != "" {
		t.Error("a bare app should not declare the legacy Bluetooth permissions")
	}

	// Enabling a feature brings exactly its permissions, and nothing else's.
	cam := resolveAndroidPermissions([]string{"goleo_camera"}, nil)
	if !hasPerm(cam.Permissions, "android.permission.CAMERA") {
		t.Errorf("Camera should declare CAMERA, got %v", cam.Permissions)
	}
	if hasPerm(cam.Permissions, "android.permission.NFC") {
		t.Error("enabling Camera must not drag in NFC")
	}
	if !hasPerm(cam.Features, "android.hardware.camera") {
		t.Errorf("Camera should declare the hardware feature, got %v", cam.Features)
	}
}

// This is the mitigation the design leans on, so it has to work: extra_permissions must
// be able to add anything detection cannot see.
func TestExtraPermissionsAreHonouredAndNormalised(t *testing.T) {
	p := resolveAndroidPermissions(nil, []string{
		"android.permission.RECORD_AUDIO", // fully qualified
		"READ_CONTACTS",                   // bare name
		"  ",                              // ignored
		"",                                // ignored
	})
	for _, want := range []string{"android.permission.RECORD_AUDIO", "android.permission.READ_CONTACTS"} {
		if !hasPerm(p.Permissions, want) {
			t.Errorf("extra_permissions did not yield %s: %v", want, p.Permissions)
		}
		if p.Explain[want] != "extra_permissions" {
			t.Errorf("%s should be attributed to extra_permissions, got %q", want, p.Explain[want])
		}
	}
	for _, p2 := range p.Permissions {
		if strings.TrimSpace(p2) == "" || !strings.Contains(p2, ".") {
			t.Errorf("emitted a malformed permission %q", p2)
		}
	}
}

// Bluetooth needs the pre-API-31 grants bounded by maxSdkVersion, or Play reports them
// as unnecessary on modern devices.
func TestBluetoothCarriesBoundedLegacyPermissions(t *testing.T) {
	p := resolveAndroidPermissions([]string{"goleo_ble"}, nil)
	if !hasPerm(p.Permissions, "android.permission.BLUETOOTH_SCAN") ||
		!hasPerm(p.Permissions, "android.permission.BLUETOOTH_CONNECT") {
		t.Errorf("Bluetooth should declare SCAN and CONNECT, got %v", p.Permissions)
	}
	if !strings.Contains(p.LegacyXML, `android:maxSdkVersion="30"`) {
		t.Errorf("legacy Bluetooth permissions need maxSdkVersion=30:\n%s", p.LegacyXML)
	}
	// The bounded ones must NOT also appear unbounded in the plain list.
	if hasPerm(p.Permissions, "android.permission.BLUETOOTH") {
		t.Error("BLUETOOTH must only appear in the maxSdkVersion-bounded XML")
	}
}

// Every permission must be attributable, since the printed report is what makes a
// detection false negative visible at build time.
func TestEveryPermissionIsAttributed(t *testing.T) {
	p := resolveAndroidPermissions([]string{"goleo_camera", "goleo_geolocation", "goleo_vibration"}, []string{"RECORD_AUDIO"})
	for _, name := range p.Permissions {
		if p.Explain[name] == "" {
			t.Errorf("%s has no attribution — the build report would show a blank source", name)
		}
	}
}

// The rendered manifest must be well-formed and contain what was resolved. Renders the
// real embedded template, so a template typo fails here rather than inside Gradle.
func TestManifestTemplateRendersResolvedPermissions(t *testing.T) {
	raw, err := mobileTemplates.ReadFile("templates/android/app/src/main/AndroidManifest.xml")
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("m").Parse(string(raw))
	if err != nil {
		t.Fatalf("the manifest template does not parse: %v", err)
	}
	cfg := mobileConfig{
		PackageName: "com.example.app",
		Perms:       resolveAndroidPermissions([]string{"goleo_camera", "goleo_ble"}, []string{"RECORD_AUDIO"}),
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, cfg); err != nil {
		t.Fatal(err)
	}
	got := out.String()

	for _, want := range []string{
		`<uses-permission android:name="android.permission.CAMERA" />`,
		`<uses-permission android:name="android.permission.RECORD_AUDIO" />`,
		`<uses-permission android:name="android.permission.INTERNET" />`,
		`android:maxSdkVersion="30"`,
		`<uses-feature android:name="android.hardware.camera" android:required="false" />`,
		`<application`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered manifest is missing %q:\n%s", want, got)
		}
	}
	// Nothing the app did not enable.
	if strings.Contains(got, "android.permission.ACCESS_FINE_LOCATION") {
		t.Errorf("rendered manifest declares location without Geolocation enabled:\n%s", got)
	}
	// Guard against an empty render, which would be a template that silently matched
	// nothing rather than one that worked.
	if strings.Count(got, "<uses-permission") < 4 {
		t.Errorf("suspiciously few permissions rendered:\n%s", got)
	}
}

// hasPerm reports whether a resolved list contains an entry. cli/cmd's existing
// contains() helper compares two strings, not a slice and a string.
func hasPerm(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// --- release flags ---

func TestResolveAndroidFormatDefaults(t *testing.T) {
	// Play only accepts .aab for new uploads, so --release must default to it; a debug
	// build defaults to .apk because that is what `adb install` takes.
	if f, _ := resolveAndroidFormat("", true); f != androidFormatAAB {
		t.Errorf("--release default = %q, want aab", f)
	}
	if f, _ := resolveAndroidFormat("", false); f != androidFormatAPK {
		t.Errorf("debug default = %q, want apk", f)
	}
	// An explicit format wins either way, including a signed APK for distribution
	// outside a store.
	if f, _ := resolveAndroidFormat("apk", true); f != androidFormatAPK {
		t.Errorf("--release --android-format apk = %q, want apk", f)
	}
	if f, _ := resolveAndroidFormat("AAB", false); f != androidFormatAAB {
		t.Errorf("case-insensitive aab failed: %q", f)
	}
	if _, err := resolveAndroidFormat("ipa", false); err == nil {
		t.Error("an unknown format should be rejected, not silently defaulted")
	}
}

// The gradle task and the artifact path have to agree, or a correct build reports
// "gradle succeeded but produced nothing".
func TestAndroidGradleTaskMatchesItsArtifact(t *testing.T) {
	cases := []struct {
		format   androidArtifactFormat
		release  bool
		wantTask string
		wantIn   string
	}{
		{androidFormatAAB, true, "bundleRelease", "app-release.aab"},
		{androidFormatAPK, true, "assembleRelease", "app-release.apk"},
		{androidFormatAPK, false, "assembleDebug", "app-debug.apk"},
		{androidFormatAAB, false, "bundleDebug", "app-debug.aab"},
	}
	for _, c := range cases {
		task, candidates := androidGradleTask(c.format, c.release)
		if task != c.wantTask {
			t.Errorf("%s/release=%v task = %q, want %q", c.format, c.release, task, c.wantTask)
		}
		if len(candidates) == 0 {
			t.Fatalf("%s/release=%v has no artifact candidates", c.format, c.release)
		}
		found := false
		for _, cand := range candidates {
			if strings.HasSuffix(cand, c.wantIn) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s/release=%v candidates %v do not include %s", c.format, c.release, candidates, c.wantIn)
		}
	}

	// assembleRelease with no signingConfig emits app-release-UNSIGNED.apk. Without
	// that candidate a legitimate --no-sign build would report "produced nothing".
	_, relCandidates := androidGradleTask(androidFormatAPK, true)
	unsigned := false
	for _, c := range relCandidates {
		if strings.Contains(c, "-unsigned") {
			unsigned = true
		}
	}
	if !unsigned {
		t.Errorf("assembleRelease candidates must include the -unsigned variant, got %v", relCandidates)
	}
}

// An unsigned release artifact cannot be uploaded or installed, so this must be an
// error rather than the "print a notice and continue" pattern desktop signing uses.
func TestReleaseWithoutKeystoreIsRefusedUnlessNoSign(t *testing.T) {
	orig := os.Getenv("GOLEO_ANDROID_KEYSTORE")
	t.Cleanup(func() {
		os.Setenv("GOLEO_ANDROID_KEYSTORE", orig)
		buildRelease, buildNoSign = false, false
	})
	os.Unsetenv("GOLEO_ANDROID_KEYSTORE")

	buildRelease, buildNoSign = true, false
	err := validateAndroidRelease()
	if err == nil {
		t.Fatal("--release with no keystore should be refused")
	}
	// The message has to say what to set and how to make one — this is the first wall
	// a developer shipping to Play hits.
	for _, want := range []string{"GOLEO_ANDROID_KEYSTORE", "keytool", "--no-sign"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should mention %q:\n%s", want, err)
		}
	}

	// --no-sign is the explicit opt out.
	buildNoSign = true
	if err := validateAndroidRelease(); err != nil {
		t.Errorf("--release --no-sign should be allowed without a keystore: %v", err)
	}

	// A configured keystore is accepted. It has to be a real file: validateAndroidRelease
	// now checks the path opens, so that a typo fails in the first second instead of
	// inside Gradle after a full build.
	buildNoSign = false
	ks := filepath.Join(t.TempDir(), "release.jks")
	if err := os.WriteFile(ks, []byte("keystore"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Setenv("GOLEO_ANDROID_KEYSTORE", ks)
	if err := validateAndroidRelease(); err != nil {
		t.Errorf("a configured keystore should validate: %v", err)
	}

	// A debug build never needs one.
	buildRelease = false
	os.Unsetenv("GOLEO_ANDROID_KEYSTORE")
	if err := validateAndroidRelease(); err != nil {
		t.Errorf("a debug build should not require signing: %v", err)
	}
}

// versionCode and versionName were hardcoded 1 and "1.0" in build.gradle.kts, so
// goleo.json's values were loaded and thrown away and every app published as the same
// version. Play rejects an upload whose versionCode has not increased.
func TestGradleTemplateTakesVersionAndSdkFromConfig(t *testing.T) {
	raw, err := mobileTemplates.ReadFile("templates/android/app/build.gradle.kts")
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("g").Parse(string(raw))
	if err != nil {
		t.Fatalf("build.gradle.kts does not parse as a template: %v", err)
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, mobileConfig{
		PackageName: "com.example.app",
		VersionName: "2.5.7", VersionCode: 20507,
		MinSDK: 26, TargetSDK: 34,
	}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"versionCode = 20507",
		`versionName = "2.5.7"`,
		"minSdk = 26",
		"targetSdk = 34",
		"compileSdk = 34",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered gradle is missing %q:\n%s", want, got)
		}
	}
	// The old hardcoded values must be gone.
	for _, gone := range []string{"versionCode = 1\n", `versionName = "1.0"`, "minSdk = 24\n"} {
		if strings.Contains(got, gone) {
			t.Errorf("rendered gradle still hardcodes %q", gone)
		}
	}
	// Signing must be read from the environment inside gradle, never passed in — a
	// keystore password on gradle's command line shows up in ps output and its logs.
	for _, want := range []string{
		`System.getenv("GOLEO_ANDROID_KEYSTORE")`,
		`System.getenv("GOLEO_ANDROID_KEYSTORE_PASSWORD")`,
		`System.getenv("GOLEO_ANDROID_KEY_ALIAS")`,
		`System.getenv("GOLEO_ANDROID_KEY_PASSWORD")`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("build.gradle.kts should read %s itself:\n%s", want, got)
		}
	}
}

// The bug this exists for: resolveAndroidPermissions matched featureRegistry.Name
// ("Camera") against what detectFeatureUsage emits, which is BUILD TAGS
// ("goleo_camera"). Nothing ever matched, so a demo project full of camera and
// Bluetooth pages resolved to three core permissions and would have shipped with its
// hardware features silently unusable.
//
// Every unit test above missed it because they hand-wrote the input. Passing "Camera"
// tested the value the code expected rather than the value the producer emits — the same
// shape of blind spot as a test that calls nsisScript with "app.exe" while production
// passes "app". So this drives the REAL scanner over a real project layout and requires
// its output to resolve to real permissions.
func TestResolveUsesTheVocabularyTheScannerEmits(t *testing.T) {
	dir := t.TempDir()
	// A project that enables camera and geolocation the way an app actually does.
	src := filepath.Join(dir, "backend", "app")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	app := `package app

import "github.com/daforester/goleo/runtime"

func New() {
	a := runtime.New(runtime.Config{})
	runtime.RegisterCamera(a.Bridge())
	runtime.RegisterGeolocation(a.Bridge())
}
`
	if err := os.WriteFile(filepath.Join(src, "app.go"), []byte(app), 0o644); err != nil {
		t.Fatal(err)
	}

	detected, err := detectFeatureUsage(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(detected) == 0 {
		t.Fatal("the scanner found nothing in a project that calls RegisterCamera — " +
			"detection is broken, so permission derivation cannot work")
	}

	p := resolveAndroidPermissions(detected, nil)
	for _, want := range []string{"android.permission.CAMERA", "android.permission.ACCESS_FINE_LOCATION"} {
		if !hasPerm(p.Permissions, want) {
			t.Errorf("scanner emitted %v but %s was not resolved — the two sides use "+
				"different vocabularies", detected, want)
		}
	}
	if !hasPerm(p.Features, "android.hardware.camera") {
		t.Errorf("camera hardware feature missing; resolved features = %v", p.Features)
	}
}

// impliedHardwareFeatures is Android's permission -> implied <uses-feature> mapping, as far
// as it affects goleo. aapt2 derives these automatically and an implied entry defaults to
// required="TRUE", so any permission here whose feature goleo does not declare explicitly
// becomes a hard device filter on Play.
//
// Confirmed against a real Play upload (2026-08-05): the listing reported six features where
// goleo declared three, the extras being android.hardware.bluetooth (from BLUETOOTH /
// BLUETOOTH_ADMIN) and android.hardware.location (from ACCESS_FINE_LOCATION) — plus
// android.hardware.faketouch, a baseline every device satisfies, which is why it is not here.
var impliedHardwareFeatures = map[string][]string{
	"android.permission.CAMERA":               {"android.hardware.camera"},
	"android.permission.NFC":                  {"android.hardware.nfc"},
	"android.permission.BLUETOOTH":            {"android.hardware.bluetooth"},
	"android.permission.BLUETOOTH_ADMIN":      {"android.hardware.bluetooth"},
	"android.permission.ACCESS_FINE_LOCATION": {"android.hardware.location"},
	"android.permission.RECORD_AUDIO":         {"android.hardware.microphone"},
}

// Every permission that implies a hardware feature must have that feature declared
// explicitly, so it lands as required="false" instead of being implied as required.
//
// This is the invariant, not the instance: enable a feature whose permission implies
// hardware, forget the <uses-feature>, and Play silently narrows who can install the app.
// Nothing about the build output would show it — the permission list looks right.
func TestEveryImpliedHardwareFeatureIsDeclaredOptional(t *testing.T) {
	// The worst case is every feature enabled, which is what the demo scaffold does.
	var allTags []string
	for _, f := range featureRegistry {
		allTags = append(allTags, f.BuildTag)
	}
	perms := resolveAndroidPermissions(allTags, nil)

	declared := map[string]bool{}
	for _, f := range perms.Features {
		declared[f] = true
	}

	// The legacy Bluetooth permissions are emitted as raw XML, so include them.
	requested := append([]string{}, perms.Permissions...)
	if strings.Contains(perms.LegacyXML, "BLUETOOTH") {
		requested = append(requested,
			"android.permission.BLUETOOTH", "android.permission.BLUETOOTH_ADMIN")
	}

	for _, perm := range requested {
		for _, feature := range impliedHardwareFeatures[perm] {
			if !declared[feature] {
				t.Errorf("%s implies <uses-feature %s>, which goleo does not declare — aapt2 "+
					"will add it as required=\"true\" and Play will hide the app from every "+
					"device lacking that hardware, even though the feature degrades gracefully",
					perm, feature)
			}
		}
	}
}

// Every feature goleo declares must be optional. One required entry is a device filter.
func TestAllDeclaredFeaturesAreOptional(t *testing.T) {
	raw, err := mobileTemplates.ReadFile("templates/android/app/src/main/AndroidManifest.xml")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	if !strings.Contains(src, `android:required="false"`) {
		t.Fatal("the manifest template does not mark <uses-feature> optional at all")
	}
	// There must be no uses-feature line without required="false".
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, "uses-feature") && !strings.Contains(line, `android:required="false"`) {
			t.Errorf("a <uses-feature> is declared without required=\"false\": %q",
				strings.TrimSpace(line))
		}
	}
}

// The permission report must not present goleo's derivation as the final answer.
//
// It is mitigation (2) for detection false negatives, so it has to be trustworthy in BOTH
// directions: something missing means the scanner did not see a Register* call, and
// something present in the artifact but absent here means Gradle's manifest merger added
// it. Measured on a minimal app that registers nothing: goleo declares 3 permissions and
// the APK ships 7 (WAKE_LOCK, RECEIVE_BOOT_COMPLETED, FOREGROUND_SERVICE and a
// DYNAMIC_RECEIVER_* permission come from library manifests). WAKE_LOCK and
// RECEIVE_BOOT_COMPLETED show on a Play listing, so a developer who trusts this output
// alone meets them at submission time.
//
// Asserted on the printed text because that text is the whole mitigation.
func TestPermissionReportAdmitsTheManifestMergerAddsMore(t *testing.T) {
	out := captureStdout(t, func() {
		reportAndroidPermissions(resolveAndroidPermissions([]string{"goleo_camera"}, nil))
	})
	for _, want := range []string{"merger", "aapt2 dump permissions"} {
		if !strings.Contains(out, want) {
			t.Errorf("the permission report should mention %q so the reader knows this is not "+
				"the final permission set:\n%s", want, out)
		}
	}
}
