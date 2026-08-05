package cmd

import (
	"regexp"
	"strings"
	"testing"
	"text/template"
)

// The iOS templates had four defects that only a real build surfaces, so they are asserted
// here where a `go test` can catch a regression on any host — this repo builds iOS only on
// a macOS runner, and only in one job.
func TestIOSTemplatesTakeTheirValuesFromConfig(t *testing.T) {
	cfg := mobileConfig{
		PackageName:         "com.example.android",
		AppName:             "My iOS App",
		VersionName:         "2.5.7",
		VersionCode:         20507,
		IOSBundleID:         "com.example.ios",
		IOSDeploymentTarget: "16.2",
	}

	render := func(path string) string {
		t.Helper()
		raw, err := mobileTemplates.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		tmpl, err := template.New("t").Parse(string(raw))
		if err != nil {
			t.Fatalf("%s does not parse as a template: %v", path, err)
		}
		var out strings.Builder
		if err := tmpl.Execute(&out, cfg); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return out.String()
	}

	plist := render("templates/ios/App/Info.plist")
	for _, want := range []string{
		"<string>My iOS App</string>", // CFBundleName, was hardcoded "Goleo App"
		"<string>2.5.7</string>",      // CFBundleShortVersionString, was "1.0"
		"<string>20507</string>",      // CFBundleVersion, was "1"
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("Info.plist is missing %s:\n%s", want, plist)
		}
	}
	for _, gone := range []string{"<string>Goleo App</string>", "<string>1.0</string>"} {
		if strings.Contains(plist, gone) {
			t.Errorf("Info.plist still hardcodes %s", gone)
		}
	}

	// The bundle id must come from mobile.ios.bundle_identifier, NOT the Android package
	// name. Using PackageName for both made the two apps indistinguishable to Apple.
	proj := render("templates/ios/xcodegen.yml")
	if !strings.Contains(proj, "com.example.ios") {
		t.Errorf("xcodegen.yml does not use the iOS bundle id:\n%s", proj)
	}
	if strings.Contains(proj, "com.example.android") {
		t.Errorf("xcodegen.yml is still using the ANDROID package name:\n%s", proj)
	}
	if !strings.Contains(proj, `iOS: "16.2"`) {
		t.Errorf("xcodegen.yml does not use mobile.ios.deployment_target:\n%s", proj)
	}
	// Without PRODUCT_NAME the product is named after the target ("App.app") while the CLI
	// reports GoleoApp.app — a path it never writes.
	if !strings.Contains(proj, "PRODUCT_NAME: GoleoApp") {
		t.Errorf("xcodegen.yml must pin PRODUCT_NAME:\n%s", proj)
	}
}

// Info.plist points UILaunchStoryboardName at "LaunchScreen". A build whose referenced
// launch storyboard is missing shows a black launch screen and is rejected by App Store
// review — and the file did not exist at all until now.
func TestLaunchScreenStoryboardExists(t *testing.T) {
	plist, err := mobileTemplates.ReadFile("templates/ios/App/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plist), "UILaunchStoryboardName") {
		t.Skip("Info.plist no longer references a launch storyboard")
	}
	if _, err := mobileTemplates.ReadFile("templates/ios/App/LaunchScreen.storyboard"); err != nil {
		t.Fatalf("Info.plist references UILaunchStoryboardName but LaunchScreen.storyboard "+
			"is not in the template: %v", err)
	}
}

// The generated project's FILE FORMAT must be pinned, not inherited from whatever XcodeGen
// the user happens to have. XcodeGen defaults to xcode16_0 = objectVersion 77, and an Xcode
// older than 16.0 refuses to open that at all:
//
//	The project 'GoleoApp' cannot be opened because it is in a future Xcode project
//	file format (77).                                  xcodebuild: exit status 74
//
// which is how the first run of the ios-simulator CI job failed. It matters beyond CI:
// goleo tells users to open this project in Xcode for device builds, so it has to be
// readable by the Xcode they have, and `brew install xcodegen` tracks the newest Xcode
// rather than theirs.
func TestGeneratedXcodeProjectFormatIsPinnedToTheMostCompatible(t *testing.T) {
	raw, err := mobileTemplates.ReadFile("templates/ios/xcodegen.yml")
	if err != nil {
		t.Fatal(err)
	}
	spec := string(raw)

	if !strings.Contains(spec, "projectFormat:") {
		t.Fatal("xcodegen.yml does not set options.projectFormat, so the generated project " +
			"takes XcodeGen's default (xcode16_0 / objectVersion 77) and will not open in " +
			"any Xcode older than 16.0")
	}
	// xcode14_0 is objectVersion 56 — the oldest format that carries what goleo generates,
	// and therefore the one the most Xcode versions can read. Newer Xcodes read old formats.
	if !strings.Contains(spec, "projectFormat: xcode14_0") {
		t.Errorf("projectFormat should be xcode14_0 (the most compatible); a newer value " +
			"excludes older Xcodes for no benefit — goleo generates only plain build settings")
	}
	// An unrecognised value silently falls back to xcode16_0/77, so a typo here reintroduces
	// the failure with no error of its own.
	valid := map[string]bool{
		"xcode14_0": true, "xcode15_0": true, "xcode15_3": true,
		"xcode16_0": true, "xcode16_3": true,
	}
	for _, line := range strings.Split(spec, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "projectFormat:") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(line, "projectFormat:"))
		if !valid[v] {
			t.Errorf("projectFormat %q is not one of XcodeGen's formats; an unrecognised "+
				"value falls back to xcode16_0 (objectVersion 77) silently", v)
		}
	}
}

// importGoleoLineRe matches the import STATEMENT, not the words. AppDelegate.swift's own
// explanatory comment contains "import Goleo", so a substring check passed even with the
// real import deleted — found by mutating it away and watching this test stay green.
var importGoleoLineRe = regexp.MustCompile(`(?m)^\s*import Goleo\s*$`)

// The iOS shell's binding names come from TWO different places, and getting them from the
// wrong one is what kept iOS from compiling at all:
//
//   - the Swift MODULE is the titlecased -o basename (Goleo.xcframework -> "Goleo"), so
//     `import Goleo`;
//   - every SYMBOL is prefixed with the titlecased Go PACKAGE name (package gomobile ->
//     "Gomobile"), so GomobileSetHomeDir, GomobileNotifierProtocol.
//
// AppDelegate.swift used "Goleo" for both, so it referenced symbols that do not exist.
// Verified against the generated Gomobile.objc.h on a macos-14 runner.
func TestIOSShellUsesTheGeneratedBindingNames(t *testing.T) {
	raw, err := mobileTemplates.ReadFile("templates/ios/App/AppDelegate.swift")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)

	// Match the IMPORT LINE, not the substring: the explanatory comment in AppDelegate.swift
	// also contains the words "import Goleo", so a Contains check passes even with the real
	// import deleted. Found by mutating the import away and seeing this test still pass.
	hasImport := importGoleoLineRe.MatchString(src)
	if !hasImport {
		t.Error("the Swift module is Goleo (from the .xcframework basename), so `import Goleo` " +
			"is required — and it is NOT the symbol prefix")
	}

	// Every symbol the shell needs, exactly as the generated header declares it.
	for _, sym := range []string{
		"GomobileSetHomeDir(", "GomobileSetNotifier(", "GomobileSetBatteryProvider(",
		"GomobileSetWakeLockProvider(", "GomobileSetSensorsProvider(",
		"GomobileSetBackgroundProvider(", "GomobileSetClipboardProvider(",
		"GomobileSetShareProvider(", "GomobileStartServer(", "GomobileStopServer(",
		"GomobileEmitSensorReading(", "GomobileEmitBackgroundSync(",
		"GomobileNotifierProtocol", "GomobileBatteryProviderProtocol",
		"GomobileWakeLockProviderProtocol", "GomobileSensorsProviderProtocol",
		"GomobileBackgroundProviderProtocol", "GomobileClipboardProviderProtocol",
		"GomobileShareProviderProtocol",
	} {
		if !strings.Contains(src, sym) {
			t.Errorf("AppDelegate.swift does not reference %s, which the generated "+
				"Gomobile.objc.h declares", sym)
		}
	}

	// And it must not go back to treating the module name as a namespace. `Goleo.setX(...)`
	// does not exist: package-level Go funcs become C functions, not methods.
	for _, bad := range []string{
		"Goleo.setHomeDir", "Goleo.startServer", "Goleo.stopServer",
		"Goleo.emitSensorReading", "Goleo.setNotifier",
	} {
		if strings.Contains(src, bad) {
			t.Errorf("AppDelegate.swift uses %s — the module name is not a namespace; "+
				"package-level Go funcs are C functions prefixed Gomobile", bad)
		}
	}

	// A C function takes no argument labels. `GomobileEmitSensorReading(t, x, y, z, ts)`,
	// never `(_, x:, y:, z:, timestamp:)`.
	for _, bad := range []string{"x: a.x", "y: a.y", "z: a.z", "timestamp: now()", "devMode: false"} {
		if strings.Contains(src, bad) {
			t.Errorf("AppDelegate.swift passes %q as a labelled argument; the gomobile "+
				"entry points are C functions and take none", bad)
		}
	}
}

// Both branches of the app-icon setting must be present.
//
// XcodeGen applies its own settingPresets to an application target, and those include
// ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon. So a `{{if .HasIcon}}` that only ADDS the
// setting cannot prevent it — with no icon configured the project still asked actool for an
// "AppIcon" set that goleo had deliberately not generated, and the build failed with
// "None of the input catalogs contained ... an app icon set named AppIcon". Since the
// scaffold ships no icon file, that was EVERY new project's first iOS build.
func TestAppIconSettingIsOverriddenWhenThereIsNoIcon(t *testing.T) {
	render := func(hasIcon bool) string {
		t.Helper()
		raw, err := mobileTemplates.ReadFile("templates/ios/xcodegen.yml")
		if err != nil {
			t.Fatal(err)
		}
		tmpl, err := template.New("x").Parse(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		cfg := defaultMobileConfig()
		cfg.HasIcon = hasIcon
		var out strings.Builder
		if err := tmpl.Execute(&out, cfg); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}

	withIcon := render(true)
	if !strings.Contains(withIcon, "ASSETCATALOG_COMPILER_APPICON_NAME: AppIcon") {
		t.Errorf("with an icon, the project must name the generated AppIcon set:\n%s", withIcon)
	}

	without := render(false)
	// The setting must be present and EMPTY, not absent: absent means XcodeGen's preset wins.
	if !strings.Contains(without, `ASSETCATALOG_COMPILER_APPICON_NAME: ""`) {
		t.Errorf("without an icon, the project must override XcodeGen's preset with an empty "+
			"ASSETCATALOG_COMPILER_APPICON_NAME; omitting it lets the preset ask actool for an "+
			"AppIcon set that does not exist:\n%s", without)
	}
	// And it must not simultaneously ask for AppIcon.
	for _, line := range strings.Split(without, "\n") {
		if strings.Contains(line, "ASSETCATALOG_COMPILER_APPICON_NAME") &&
			strings.Contains(line, "AppIcon") && !strings.Contains(line, "#") {
			t.Errorf("without an icon the project still names AppIcon: %q", strings.TrimSpace(line))
		}
	}
}

// The BGTask identifier the shell registers must match what Info.plist permits.
//
// BGTaskScheduler.register with an identifier absent from
// BGTaskSchedulerPermittedIdentifiers raises an NSException, and registerTask() runs first
// thing in didFinishLaunching — so a mismatch is a crash on launch, not a degraded feature.
//
// Info.plist permits $(PRODUCT_BUNDLE_IDENTIFIER).sync, which is the iOS bundle id, while
// AppDelegate.swift used the ANDROID package name. That was harmless only while the iOS build
// reused the Android id for everything; making mobile.ios.bundle_identifier take effect is
// what turned it into a crash for any project that sets one.
func TestBackgroundTaskIDMatchesTheBundleIdentifier(t *testing.T) {
	cfg := defaultMobileConfig()
	cfg.PackageName = "com.example.android"
	cfg.IOSBundleID = "com.example.ios"

	raw, err := mobileTemplates.ReadFile("templates/ios/App/AppDelegate.swift")
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("s").Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, cfg); err != nil {
		t.Fatal(err)
	}
	src := out.String()

	if !strings.Contains(src, `let backgroundSyncTaskID = "com.example.ios.sync"`) {
		t.Errorf("the BGTask identifier must derive from the iOS bundle id, since Info.plist "+
			"permits $(PRODUCT_BUNDLE_IDENTIFIER).sync; registering an unpermitted identifier "+
			"raises an NSException at launch:\n%s",
			firstMatchingLine(src, "backgroundSyncTaskID ="))
	}
	if strings.Contains(src, `let backgroundSyncTaskID = "com.example.android.sync"`) {
		t.Error("the BGTask identifier is using the ANDROID package name; on any project that " +
			"sets mobile.ios.bundle_identifier the app will crash on launch")
	}

	// And Info.plist must still be the one declaring it, from the build-setting form.
	plist, err := mobileTemplates.ReadFile("templates/ios/App/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plist), "$(PRODUCT_BUNDLE_IDENTIFIER).sync") {
		t.Error("Info.plist no longer permits $(PRODUCT_BUNDLE_IDENTIFIER).sync — if that " +
			"changed, backgroundSyncTaskID must change with it")
	}
}

func firstMatchingLine(s, needle string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimSpace(line)
		}
	}
	return "(not found)"
}
