package cmd

import (
	"strings"
	"testing"
)

// resetBuildFlags puts every build flag back to its zero value. The flags are package
// globals, so a test that sets one leaks into the next.
func resetBuildFlags(t *testing.T) {
	t.Helper()
	save := struct {
		bundle, publish, release, noSign, simulator bool
		androidFormat, windowsFormat, arch, iosTgt  string
		abi, ndk                                    string
		versionCode, androidAPI                     int
	}{
		buildBundle, buildPublish, buildRelease, buildNoSign, buildSimulator,
		buildAndroidFormat, buildWindowsFormat, buildArch, iosDeployTarget,
		buildAndroidABI, buildAndroid,
		buildVersionCode, androidAPI,
	}
	t.Cleanup(func() {
		buildBundle, buildPublish, buildRelease = save.bundle, save.publish, save.release
		buildNoSign, buildSimulator = save.noSign, save.simulator
		buildAndroidFormat, buildWindowsFormat = save.androidFormat, save.windowsFormat
		buildArch, iosDeployTarget = save.arch, save.iosTgt
		buildVersionCode, androidAPI = save.versionCode, save.androidAPI
		buildAndroidABI, buildAndroid = save.abi, save.ndk
	})
	buildBundle, buildPublish, buildRelease, buildNoSign, buildSimulator = false, false, false, false, false
	buildAndroidFormat, buildWindowsFormat, buildArch, iosDeployTarget = "", "", "", ""
	buildVersionCode, androidAPI = 0, 0
	buildAndroidABI, buildAndroid = "", ""
}

// Every build flag against every target. A flag a target cannot honour must be REFUSED,
// because the alternative is what actually shipped: `goleo build ios --release` was
// accepted and then ignored, so you asked for a release artifact, got a debug build, and
// the build reported success.
//
// Deliberately exhaustive rather than a list of known-bad pairs: a new flag that nobody
// wires into validateTargetFlags shows up here as an accepted no-op.
func TestEveryFlagIsEitherHonouredOrRefusedPerTarget(t *testing.T) {
	flags := []struct {
		name string
		set  func()
		// alsoSets names any other flag set() turns on. A combination can legitimately be
		// refused for the other flag first (--windows-format requires --bundle, and
		// --bundle is itself refused on mobile), so the message may name either.
		alsoSets []string
		// honouredBy lists the targets that actually read the flag. Everything else must
		// be refused.
		honouredBy []string
	}{
		{"--bundle", func() { buildBundle = true }, nil, []string{"current", "windows", "linux", "darwin"}},
		{"--publish", func() { buildPublish = true }, nil, []string{"current", "windows", "linux", "darwin"}},
		{"--simulator", func() { buildSimulator = true }, nil, []string{"ios"}},
		{"--release", func() { buildRelease = true }, nil, []string{"android"}},
		{"--android-format", func() { buildAndroidFormat = "aab" }, nil, []string{"android"}},
		{"--version-code", func() { buildVersionCode = 42 }, nil, []string{"android"}},
		{"--android-abi", func() { buildAndroidABI = "arm64-v8a" }, nil, []string{"android"}},
		// --android-ndk was declared on `build` and `emulate`, documented in --help, and
		// read by nothing at all: NDK resolution went straight to ANDROID_NDK_HOME and
		// autodiscovery, so naming an NDK silently used a different one.
		{"--android-ndk", func() { buildAndroid = "/opt/ndk" }, nil, []string{"android"}},
		// --windows-format additionally requires --bundle, so set both. Only a Windows
		// target reads it; "current" depends on the host, so it is checked separately.
		{"--windows-format", func() { buildWindowsFormat, buildBundle = "msix", true },
			[]string{"--bundle"}, []string{"windows", "current"}},
	}

	targetNames := []string{"current", "windows", "linux", "darwin", "android", "ios", "pwa"}

	for _, f := range flags {
		honoured := map[string]bool{}
		for _, n := range f.honouredBy {
			honoured[n] = true
		}
		for _, name := range targetNames {
			t.Run(f.name+"/"+name, func(t *testing.T) {
				resetBuildFlags(t)
				f.set()

				target := targets[name]
				err := validateTargetFlags(name, target)

				if honoured[name] {
					// "current" only honours --windows-format on a Windows host.
					if f.name == "--windows-format" && name == "current" && target.GOOS != "windows" {
						if err == nil {
							t.Errorf("--windows-format on a %s host should be refused", target.GOOS)
						}
						return
					}
					if err != nil {
						t.Errorf("%s is implemented for %s but was refused: %v", f.name, name, err)
					}
					return
				}
				if err == nil {
					t.Errorf("%s is NOT implemented for the %s target but was accepted — "+
						"it will silently do nothing and the build will report success",
						f.name, name)
					return
				}
				// The message must name one of the flags actually passed, or the user
				// cannot tell which one to drop.
				named := strings.Contains(err.Error(), f.name)
				for _, other := range f.alsoSets {
					named = named || strings.Contains(err.Error(), other)
				}
				if !named {
					t.Errorf("%s on %s was refused, but the error names none of the flags "+
						"that were passed:\n%v", f.name, name, err)
				}
			})
		}
	}
}

// With no flags set, no target may be refused — otherwise a plain `goleo build <target>`
// is broken.
func TestNoFlagsIsAlwaysAccepted(t *testing.T) {
	resetBuildFlags(t)
	for name, target := range targets {
		if err := validateTargetFlags(name, target); err != nil {
			t.Errorf("plain `goleo build %s` was refused: %v", name, err)
		}
	}
}

// --release on iOS must say why, not just "wrong target". A user asking for a release iOS
// build wants to know there is no .ipa path yet and what to do instead; "only applies to
// the android target" reads like a typo on their part.
func TestIOSReleaseRefusalExplainsItself(t *testing.T) {
	resetBuildFlags(t)
	buildRelease = true
	err := validateTargetFlags("ios", targets["ios"])
	if err == nil {
		t.Fatal("--release on ios must be refused")
	}
	for _, want := range []string{".ipa", "--simulator", "Xcode"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the iOS --release refusal should mention %q:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), "only applies to the android target") {
		t.Errorf("iOS got the generic android message instead of its own:\n%v", err)
	}
}

// --windows-format without --bundle produces no installer to format, so it must be
// refused rather than ignored.
func TestWindowsFormatNeedsBundle(t *testing.T) {
	resetBuildFlags(t)
	buildWindowsFormat = "msix"
	err := validateTargetFlags("windows", targets["windows"])
	if err == nil {
		t.Fatal("--windows-format without --bundle must be refused")
	}
	if !strings.Contains(err.Error(), "--bundle") {
		t.Errorf("the error should say --bundle is required:\n%v", err)
	}
}

// --no-sign is deliberately NOT in the table above, and this records why so it does not
// get "fixed" into a refusal. Every other flag asks for something to HAPPEN, so a target
// that ignores it leaves the user with an artifact they did not ask for. --no-sign asks
// for something NOT to happen; on a target with no signing step, that is satisfied. There
// is nothing silently wrong to report.
func TestNoSignIsAcceptedEverywhere(t *testing.T) {
	resetBuildFlags(t)
	buildNoSign = true
	for name, target := range targets {
		if err := validateTargetFlags(name, target); err != nil {
			t.Errorf("--no-sign on %s was refused: %v", name, err)
		}
	}
}
