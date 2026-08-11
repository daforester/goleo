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
		"GomobileSetShareProvider(", "GomobileSetDialogsProvider(",
		"GomobileSetMicrophoneProvider(",
		"GomobileStartServer(", "GomobileStopServer(",
		"GomobileEmitSensorReading(", "GomobileEmitBackgroundSync(",
		"GomobileNotifierProtocol", "GomobileBatteryProviderProtocol",
		"GomobileWakeLockProviderProtocol", "GomobileSensorsProviderProtocol",
		"GomobileBackgroundProviderProtocol", "GomobileClipboardProviderProtocol",
		"GomobileShareProviderProtocol", "GomobileDialogsProviderProtocol",
		"GomobileMicrophoneProviderProtocol",
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

// DEVELOPMENT_TEAM must appear only when one is configured. Emitting it unconditionally
// would write `DEVELOPMENT_TEAM: ` into every project, including Simulator builds that do
// not sign at all — and an empty team is not the same as no team to xcodebuild.
func TestXcodegenEmitsTheDevelopmentTeamOnlyWhenSet(t *testing.T) {
	render := func(team string) string {
		t.Helper()
		raw, err := mobileTemplates.ReadFile("templates/ios/xcodegen.yml")
		if err != nil {
			t.Fatal(err)
		}
		tmpl, err := template.New("t").Parse(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		cfg := mobileConfig{
			IOSBundleID:         "com.example.ios",
			IOSDeploymentTarget: "16.2",
			IOSDevelopmentTeam:  team,
		}
		var out strings.Builder
		if err := tmpl.Execute(&out, cfg); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}

	withTeam := render("ABCDE12345")
	if !strings.Contains(withTeam, "DEVELOPMENT_TEAM: ABCDE12345") {
		t.Errorf("mobile.ios.development_team did not reach the project, so a device build "+
			"still cannot be signed:\n%s", withTeam)
	}
	if !strings.Contains(withTeam, "CODE_SIGN_STYLE: Automatic") {
		t.Error("a configured team should use automatic signing; manual signing needs a " +
			"provisioning profile goleo does not manage")
	}

	withoutTeam := render("")
	if strings.Contains(withoutTeam, "DEVELOPMENT_TEAM") {
		t.Errorf("an unset team must emit no DEVELOPMENT_TEAM key at all:\n%s", withoutTeam)
	}
}

// Each of these is a defect the 2026-08-09 device spike found, in the form that would
// bring it back. They are string assertions on a template because that is the only check
// this repo can run off a Mac — the real verification is a device run.
func TestIOSShellFixesFromTheDeviceSpike(t *testing.T) {
	raw, err := mobileTemplates.ReadFile("templates/ios/App/AppDelegate.swift")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)

	// Notifications: the permission prompt appeared and nothing was ever delivered,
	// because iOS suppresses a foreground notification unless a delegate asks for it.
	// GoleoNotifier posts with `trigger: nil`, so every notification is a foreground one.
	if !strings.Contains(src, "UNUserNotificationCenter.current().delegate = self") {
		t.Error("AppDelegate must set itself as the UNUserNotificationCenter delegate, or " +
			"notifications are silently discarded while the app is in the foreground")
	}
	if !strings.Contains(src, "willPresent notification") {
		t.Error("AppDelegate must implement willPresent; without it the default is to " +
			"present nothing")
	}

	// The share sheet: UIApplication.shared.windows is deprecated since iOS 15 and empty
	// under the scene lifecycle, so the presenter resolved to nil and present() was a
	// silent no-op — no sheet, no error, and ShareProvider.share returns void so Go never
	// found out.
	// Checked against CODE only: GoleoUI's explanatory comment names the deprecated API
	// too, so a plain Contains passes with the fix reverted. The same trap as the
	// `import Goleo` assertion above, which was found by mutating the fix away.
	for i, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if strings.Contains(line, "UIApplication.shared.windows") {
			t.Errorf("line %d uses UIApplication.shared.windows, which returns an empty "+
				"array under the scene lifecycle — resolve the presenter through "+
				"GoleoUI.topViewController() instead:\n%s", i+1, line)
		}
	}
	if !strings.Contains(src, "GoleoUI.topViewController()") {
		t.Error("anything that presents must resolve its presenter via GoleoUI.topViewController()")
	}
}

// The UIScene lifecycle has to be adopted in TWO files that cannot see each other, and the
// failure mode of a mismatch is a black screen at launch with no build-time signal.
//
// Info.plist names the delegate as a STRING; the class lives in Swift. If the string is
// wrong — a renamed class, a PRODUCT_NAME change breaking $(PRODUCT_MODULE_NAME), a missing
// module prefix — UIKit finds no delegate, the scene connects to nothing and the app shows a
// black screen. AppDelegate.configurationForConnecting therefore sets delegateClass in code
// as well, so the string is a declaration with a real guarantee behind it. This test
// requires all three parts, because any two of them look complete.
func TestSceneLifecycleIsAdoptedInBothFiles(t *testing.T) {
	plistRaw, err := mobileTemplates.ReadFile("templates/ios/App/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	swiftRaw, err := mobileTemplates.ReadFile("templates/ios/App/AppDelegate.swift")
	if err != nil {
		t.Fatal(err)
	}
	plist, swift := string(plistRaw), string(swiftRaw)

	if !strings.Contains(plist, "<key>UIApplicationSceneManifest</key>") {
		t.Fatal("Info.plist declares no UIApplicationSceneManifest, so the app runs the " +
			"legacy app-delegate lifecycle: \"UIScene lifecycle will soon be required. " +
			"Failure to adopt will result in an assert in the future.\"")
	}
	// The module prefix is not optional for a Swift class, and it is the part most likely to
	// be dropped by someone copying the class name out of the .swift file.
	if !strings.Contains(plist, "<string>$(PRODUCT_MODULE_NAME).SceneDelegate</string>") {
		t.Error("UISceneDelegateClassName must be $(PRODUCT_MODULE_NAME).SceneDelegate — a " +
			"bare class name does not resolve for a Swift class, and a hardcoded module " +
			"name breaks if PRODUCT_NAME changes")
	}
	if !strings.Contains(swift, "class SceneDelegate") {
		t.Error("Info.plist names a SceneDelegate that AppDelegate.swift does not define")
	}
	if !strings.Contains(swift, "func scene(") || !strings.Contains(swift, "willConnectTo") {
		t.Error("SceneDelegate must implement scene(_:willConnectTo:options:) — nothing " +
			"else creates the window once the scene manifest exists")
	}
	if !strings.Contains(swift, "configuration.delegateClass = SceneDelegate.self") {
		t.Error("configurationForConnecting must set delegateClass, so a plist string that " +
			"fails to resolve cannot black-screen the app")
	}
	// The configuration name is the key the two sides agree on.
	if !strings.Contains(plist, "<string>Default Configuration</string>") ||
		!strings.Contains(swift, `name: "Default Configuration"`) {
		t.Error("the UISceneConfigurationName in Info.plist and the name passed to " +
			"UISceneConfiguration must be the same string")
	}
	// The window is per-scene now. An app-delegate window created unconditionally would be
	// ignored by UIKit and would hand GoleoUI a window nothing is presenting in.
	for i, line := range strings.Split(swift, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if strings.Contains(line, "UIWindow(frame:") && !strings.Contains(line, "legacyWindow") {
			t.Errorf("line %d builds a UIWindow from a frame outside the pre-iOS-13 "+
				"fallback; under the scene lifecycle it must be UIWindow(windowScene:):\n%s",
				i+1, line)
		}
	}
}

// An iPad-capable app that does not require full screen must declare all four orientations
// for iPad, or xcodebuild warns "All interface orientations must be supported unless the app
// requires full screen" (2026-08-10 device build) and App Store review flags the same thing.
//
// The condition is XcodeGen's, not goleo's: its application preset sets
// TARGETED_DEVICE_FAMILY = "1,2", so every generated project is iPad-capable whether or not
// anyone asked for it. Setting UIRequiresFullScreen would also silence the warning, by opting
// out of Split View — so this test pins WHICH of the two answers is in force, and fails if a
// future edit trims the iPad list back without taking the other route.
func TestIPadSupportsEveryOrientationOrRequiresFullScreen(t *testing.T) {
	raw, err := mobileTemplates.ReadFile("templates/ios/App/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	info := string(raw)

	// The other valid answer. Matched as a KEY, not as text: the plist explains the choice
	// in a comment that names the alternative, and a plain Contains would read that as the
	// alternative being taken.
	if strings.Contains(info, "<key>UIRequiresFullScreen</key>") {
		return
	}

	ipad := strings.Index(info, "<key>UISupportedInterfaceOrientations~ipad</key>")
	if ipad < 0 {
		t.Fatal("Info.plist declares no UISupportedInterfaceOrientations~ipad. The iPhone " +
			"list is a subset, so iPad inherits it and the build warns that all interface " +
			"orientations must be supported unless the app requires full screen")
	}
	end := strings.Index(info[ipad:], "</array>")
	if end < 0 {
		t.Fatal("UISupportedInterfaceOrientations~ipad is not followed by an array")
	}
	block := info[ipad : ipad+end]
	for _, orientation := range []string{
		"UIInterfaceOrientationPortrait<",
		"UIInterfaceOrientationPortraitUpsideDown",
		"UIInterfaceOrientationLandscapeLeft",
		"UIInterfaceOrientationLandscapeRight",
	} {
		if !strings.Contains(block, orientation) {
			t.Errorf("the iPad orientation list omits %s; all four are required unless the "+
				"app requires full screen", strings.TrimSuffix(orientation, "<"))
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
