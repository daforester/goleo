package cmd

import (
	"strings"
	"testing"
)

// The native shells decide camera / microphone / geolocation for the WebView, and nothing
// in them restricts navigation — so whatever page the WebView reaches is what these gates
// answer for. They had three separate holes:
//
//  1. iOS granted BOTH media capture and geolocation unconditionally, ignoring `origin`.
//  2. Android gated media capture with a string PREFIX, and
//     "http://127.0.0.1.evil.com/".startsWith("http://127.0.0.1") is true — an ordinary
//     registrable domain that anyone can own.
//  3. Android's geolocation callback had no origin check at all.
//
// The rule, matching devOriginAllowed() in runtime/server.go: parse the origin and compare
// the HOST for equality.
func TestWebViewPermissionsAreGatedOnTheParsedOrigin(t *testing.T) {
	swift, err := mobileTemplates.ReadFile("templates/ios/App/AppDelegate.swift")
	if err != nil {
		t.Fatal(err)
	}
	src := string(swift)

	if !strings.Contains(src, "isAppOrigin(origin) ? .grant : .deny") {
		t.Error("both iOS permission callbacks must decide on the origin; granting " +
			"unconditionally hands the camera, mic and location to any page the WebView reaches")
	}
	// Code only — the explanatory comments name the old behaviour, so a plain Contains
	// check would pass with the fix reverted.
	for i, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if strings.Contains(line, "decisionHandler(.grant)") {
			t.Errorf("line %d grants a WebView permission unconditionally:\n%s", i+1, line)
		}
	}

	for _, variant := range []string{"android", "android-dev"} {
		java, err := mobileTemplates.ReadFile(
			"templates/" + variant + "/app/src/main/java/com/goleo/app/MainActivity.java")
		if err != nil {
			t.Fatal(err)
		}
		j := string(java)

		// A prefix test on an origin is the bug, in either shell's spelling. Code lines
		// only: the helper's comment quotes the old pattern to explain why it was wrong,
		// and a whole-file Contains would flag that instead of a real regression.
		for i, line := range strings.Split(j, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if strings.Contains(line, `startsWith("http://`) {
				t.Errorf("%s line %d matches an origin by prefix, which also accepts "+
					"http://127.0.0.1.evil.com — compare the parsed host instead:\n%s",
					variant, i+1, line)
			}
		}
		if !strings.Contains(j, "private boolean isAppOrigin(String origin)") {
			t.Errorf("%s has no isAppOrigin helper to gate WebView permissions on", variant)
		}
		// Both callbacks, not just the camera one.
		if strings.Count(j, "isAppOrigin(") < 3 {
			t.Errorf("%s must gate BOTH onPermissionRequest and "+
				"onGeolocationPermissionsShowPrompt on the origin (found %d uses of "+
				"isAppOrigin, expected the helper plus two call sites)",
				variant, strings.Count(j, "isAppOrigin("))
		}
	}
}

// The dev shell additionally accepts 10.0.2.2 — the Android emulator's alias for the host
// machine's loopback, which is the dev server's real origin. The release shell must NOT,
// or a shipped app trusts a private-network address it has no reason to.
func TestOnlyTheDevShellTrustsTheEmulatorHostAlias(t *testing.T) {
	release, err := mobileTemplates.ReadFile(
		"templates/android/app/src/main/java/com/goleo/app/MainActivity.java")
	if err != nil {
		t.Fatal(err)
	}
	dev, err := mobileTemplates.ReadFile(
		"templates/android-dev/app/src/main/java/com/goleo/app/MainActivity.java")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dev), `"10.0.2.2".equals(host)`) {
		t.Error("the dev shell must accept 10.0.2.2, or camera/geolocation break when the " +
			"emulator reaches the dev server through the host alias")
	}
	if strings.Contains(string(release), `"10.0.2.2".equals(host)`) {
		t.Error("the release shell must not trust 10.0.2.2 — that is a dev-only origin")
	}
}
