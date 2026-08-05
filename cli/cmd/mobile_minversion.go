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

// validIOSVersion accepts "15" / "15.0" / "15.4.1" and nothing else. gomobile passes the
// value straight to clang's -miphoneos-version-min, where a malformed value produces an
// error from deep inside the toolchain that does not mention goleo.json.
func validIOSVersion(v string) error {
	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		return fmt.Errorf("not an iOS version (want MAJOR[.MINOR[.PATCH]])")
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return fmt.Errorf("not an iOS version (want MAJOR[.MINOR[.PATCH]])")
		}
		if n < 0 {
			return fmt.Errorf("not an iOS version (want MAJOR[.MINOR[.PATCH]])")
		}
		// iOS 9 predates gomobile's own floor and 99 is beyond anything real. The range
		// deliberately stops there: a major in the thirties reads like an Android API
		// level, but Apple's renumbering to iOS 26 makes it a plausible version too, so
		// narrowing further would reject a legitimate target to catch a typo.
		if i == 0 && (n < 9 || n > 99) {
			return fmt.Errorf("iOS %d is not a plausible deployment target", n)
		}
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
