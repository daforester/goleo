package cmd

import (
	"fmt"
	"strconv"
	"strings"
)

// The minimum OS version a mobile build targets is consumed in TWO places that must
// agree: gomobile builds the Go library against it (`-iosversion` / `-androidapi`), and
// the native project declares it (`deploymentTarget` in xcodegen.yml, `minSdk` in
// build.gradle.kts). Each had its own independent source — a CLI flag for gomobile and
// goleo.json for the project — so they could disagree, and on iOS they disagreed even
// with no configuration at all: `--ios-target` defaulted to 14.0 while
// mobile.ios.deployment_target defaulted to 15.0.
//
// A framework whose minimum is BELOW the app's is harmless, which is why the mismatched
// defaults never showed up. The failing direction is a framework minimum ABOVE the app's:
// set mobile.ios.deployment_target to "13.0" and Xcode refuses the link — "building for
// iOS 13.0 but goleo.xcframework was built for iOS 14.0" — naming a version the user
// never chose. The same applies to `min_sdk` below 24 on Android.
//
// So resolve one value from goleo.json and let the flag override it explicitly, rather
// than having the flag's default silently compete with the config.

// resolveIOSMinVersion returns the iOS version to build the Go framework against.
// flagValue is --ios-target ("" when not given); configValue is the already-defaulted
// mobile.ios.deployment_target.
func resolveIOSMinVersion(flagValue, configValue string) (string, error) {
	if v := strings.TrimSpace(flagValue); v != "" {
		if err := validIOSVersion(v); err != nil {
			return "", fmt.Errorf("--ios-target %q: %w", v, err)
		}
		return v, nil
	}
	if v := strings.TrimSpace(configValue); v != "" {
		if err := validIOSVersion(v); err != nil {
			return "", fmt.Errorf("goleo.json: mobile.ios.deployment_target %q: %w", v, err)
		}
		return v, nil
	}
	return defaultIOSDeployTarget, nil
}

// goleo's iOS floor is the lowest version on which the generated shell actually WORKS,
// not the lowest one that builds. Those are different numbers, and every version between
// them produces an app that compiles, signs, installs, launches — and is missing features
// it advertises, with nothing in the build output saying so.
//
// The binding constraint is WKWebView permission delegation. iOS registers nine providers
// and camera and geolocation are not among them, so on iOS those two features reach the
// hardware ONLY through the WebView's web APIs, which WebKit denies unless the app answers
// the matching WKUIDelegate callback:
//
//	requestMediaCapturePermissionFor   iOS 15.0    camera + microphone via getUserMedia
//	requestGeolocationPermissionFor    iOS 15.4    navigator.geolocation
//
// Both are marked @available in AppDelegate.swift, so a lower deployment target compiles
// cleanly and the method is simply never called. The demo's own registry.ts claims
// ios:'yes' for camera and geolocation, and below 15.4 that claim is false.
//
// Earlier floors, kept here because each is a real cliff and the reason is not guessable
// from the source:
//
//	13.0   UIApplicationSceneManifest + SceneDelegate. Below it the scene manifest is
//	       ignored, nothing creates a window, and the app launches to a BLACK SCREEN.
//	14.0   UNNotificationPresentationOptions.banner — this one has a real fallback
//	       (.alert), so it does not constrain the floor.
//
// So 15.4 it is: the first version where nothing silently missing. Raising this floor is a
// deliberate trade — it excludes iOS 15.0–15.3 — and it is the right one, because the
// alternative is shipping an app whose camera and location pages fail with no diagnosis.
const (
	iosFloorMajor = 15
	iosFloorMinor = 4
)

// validIOSVersion accepts "15.4" / "15.4.1" / "16" and nothing below the floor above.
// gomobile passes the value straight to clang's -miphoneos-version-min, where a malformed
// value produces an error from deep inside the toolchain that does not mention goleo.json.
func validIOSVersion(v string) error {
	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		return fmt.Errorf("not an iOS version (want MAJOR[.MINOR[.PATCH]])")
	}
	nums := make([]int, 0, 3)
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return fmt.Errorf("not an iOS version (want MAJOR[.MINOR[.PATCH]])")
		}
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		return fmt.Errorf("not an iOS version (want MAJOR[.MINOR[.PATCH]])")
	}

	// 99 is beyond anything real. The range deliberately stops there: a major in the
	// thirties reads like an Android API level, but Apple's renumbering to iOS 26 makes it
	// a plausible version too, so narrowing further would reject a legitimate target to
	// catch a typo.
	if nums[0] > 99 {
		return fmt.Errorf("iOS %d is not a plausible deployment target", nums[0])
	}

	// An omitted minor is 0 — "15" means 15.0, which is below 15.4.
	minor := 0
	if len(nums) > 1 {
		minor = nums[1]
	}
	if nums[0] < iosFloorMajor || (nums[0] == iosFloorMajor && minor < iosFloorMinor) {
		return fmt.Errorf("iOS %s is below goleo's floor of %d.%d. The generated shell needs "+
			"iOS 13 for the UIScene lifecycle (below it nothing creates a window and the app "+
			"launches to a black screen), 15.0 to grant the WebView camera and microphone "+
			"access, and 15.4 to grant it geolocation — iOS registers no native provider for "+
			"camera or geolocation, so the WebView is the only path to either. A lower target "+
			"builds and signs cleanly and then ships those features silently broken",
			v, iosFloorMajor, iosFloorMinor)
	}
	return nil
}

// resolveAndroidMinAPI returns the Android API level to build the Go library against.
// flagValue is --android-api (0 when not given); configValue is mobile.android.min_sdk
// (0 when unset).
func resolveAndroidMinAPI(flagValue, configValue int) (int, error) {
	if flagValue != 0 {
		if err := validAndroidAPI(flagValue); err != nil {
			return 0, fmt.Errorf("--android-api %d: %w", flagValue, err)
		}
		return flagValue, nil
	}
	if configValue != 0 {
		if err := validAndroidAPI(configValue); err != nil {
			return 0, fmt.Errorf("goleo.json: mobile.android.min_sdk %d: %w", configValue, err)
		}
		return configValue, nil
	}
	return defaultAndroidMinSDK, nil
}

func validAndroidAPI(n int) error {
	if n < 21 || n > 99 {
		return fmt.Errorf("out of range (21-99)")
	}
	return nil
}
