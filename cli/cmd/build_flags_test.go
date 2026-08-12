package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// resetBuildFlags puts every build flag back to its zero value. The flags are package
// globals, so a test that sets one leaks into the next.
func resetBuildFlags(t *testing.T) {
	t.Helper()
	save := struct {
		bundle, publish, release, noSign, simulator bool
		androidFormat, windowsFormat, arch, iosTgt  string
		abi, ndk, iosTeamID                         string
		versionCode, androidAPI                     int
	}{
		buildBundle, buildPublish, buildRelease, buildNoSign, buildSimulator,
		buildAndroidFormat, buildWindowsFormat, buildArch, iosDeployTarget,
		buildAndroidABI, buildAndroid, iosTeam,
		buildVersionCode, androidAPI,
	}
	t.Cleanup(func() {
		buildBundle, buildPublish, buildRelease = save.bundle, save.publish, save.release
		buildNoSign, buildSimulator = save.noSign, save.simulator
		buildAndroidFormat, buildWindowsFormat = save.androidFormat, save.windowsFormat
		buildArch, iosDeployTarget = save.arch, save.iosTgt
		buildVersionCode, androidAPI = save.versionCode, save.androidAPI
		buildAndroidABI, buildAndroid = save.abi, save.ndk
		iosTeam = save.iosTeamID
	})
	buildBundle, buildPublish, buildRelease, buildNoSign, buildSimulator = false, false, false, false, false
	buildAndroidFormat, buildWindowsFormat, buildArch, iosDeployTarget = "", "", "", ""
	buildVersionCode, androidAPI = 0, 0
	buildAndroidABI, buildAndroid, iosTeam = "", "", ""
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
		// These three were declared globally and read by exactly one target each, so every
		// other target accepted them and silently ignored them.
		{"--ios-target", func() { iosDeployTarget = "17.0" }, nil, []string{"ios"}},
		{"--ios-team", func() { iosTeam = "ABCDE12345" }, nil, []string{"ios"}},
		{"--android-api", func() { androidAPI = 30 }, nil, []string{"android"}},
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

// --ios-team is honoured by the ios target but cannot apply to a Simulator build, which is
// unsigned by design. Refused rather than ignored — and the refusal must not read as "wrong
// target", which would send the user looking for a mistake they did not make.
func TestIOSTeamIsRefusedWithSimulator(t *testing.T) {
	resetBuildFlags(t)
	iosTeam, buildSimulator = "ABCDE12345", true
	err := validateTargetFlags("ios", targets["ios"])
	if err == nil {
		t.Fatal("--ios-team with --simulator must be refused: a Simulator build is not signed")
	}
	if !strings.Contains(err.Error(), "--simulator") {
		t.Errorf("the refusal should name --simulator as the reason:\n%v", err)
	}

	// And each on its own must still be fine, or the check has over-reached.
	resetBuildFlags(t)
	iosTeam = "ABCDE12345"
	if err := validateTargetFlags("ios", targets["ios"]); err != nil {
		t.Errorf("--ios-team alone on ios was refused: %v", err)
	}
	resetBuildFlags(t)
	buildSimulator = true
	if err := validateTargetFlags("ios", targets["ios"]); err != nil {
		t.Errorf("--simulator alone on ios was refused: %v", err)
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

// Flags that were declared and read by nothing. Each was accepted silently, so a script
// passing it believed it had an effect. Kept as a list because the failure mode is
// re-adding one "for symmetry" with another command.
func TestRemovedDeadFlagsStayRemoved(t *testing.T) {
	cases := []struct {
		cmd    string
		flag   string
		reason string
	}{
		{"emulate", "output", "the dev APK is installed from .goleo/ and never copied out, " +
			"so there is no artifact for a name to apply to"},
	}
	commands := map[string]*cobra.Command{"emulate": emulateCmd, "build": buildCmd}
	for _, c := range cases {
		cmd, ok := commands[c.cmd]
		if !ok {
			t.Fatalf("no such command in the test map: %s", c.cmd)
		}
		if f := cmd.Flags().Lookup(c.flag); f != nil {
			t.Errorf("`goleo %s --%s` was removed because %s; if it is back, it must actually "+
				"be read", c.cmd, c.flag, c.reason)
		}
	}
}

// And the ones that ARE declared must be honoured. --android-ndk is the cautionary case:
// declared on both build and emulate, documented, read by nothing.
func TestAndroidNDKFlagIsDeclaredOnBothCommands(t *testing.T) {
	for name, cmd := range map[string]*cobra.Command{"build": buildCmd, "emulate": emulateCmd} {
		if cmd.Flags().Lookup("android-ndk") == nil {
			t.Errorf("`goleo %s` no longer declares --android-ndk", name)
		}
	}
}

// snapshotModFiles is the guard against the "inconsistent vendoring" failure that shipped
// in v0.8.1-0.8.8 and was found by a user in production: the mobile path adds build-only
// golang.org/x/mobile deps to go.mod under -mod=mod and never re-vendors, so without a
// restore the next desktop build (-mod=vendor, because the scaffold commits vendor/) fails
// with a vendoring message that names neither the mobile build nor the polluted file.
func TestSnapshotModFilesRestoresWhatTheMobileBuildChanges(t *testing.T) {
	dir := t.TempDir()
	gomod := filepath.Join(dir, "go.mod")
	gosum := filepath.Join(dir, "go.sum")
	const originalMod = "module example.com/app\n\ngo 1.24\n"
	const originalSum = "example.com/dep v1.0.0 h1:abc=\n"
	if err := os.WriteFile(gomod, []byte(originalMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gosum, []byte(originalSum), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := snapshotModFiles(dir)

	// Stand in for what `go get -tool` + `gomobile bind` do.
	polluted := originalMod + "\nrequire golang.org/x/mobile v0.0.0-20240101000000-abcdef123456\n"
	if err := os.WriteFile(gomod, []byte(polluted), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gosum, []byte(originalSum+"golang.org/x/mobile v0.0.0 h1:xyz=\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore()

	for path, want := range map[string]string{gomod: originalMod, gosum: originalSum} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s was not restored:\n got: %q\nwant: %q", filepath.Base(path), got, want)
		}
		if strings.Contains(string(got), "golang.org/x/mobile") {
			t.Errorf("%s still lists golang.org/x/mobile — vendor/ is now inconsistent and the "+
				"next desktop build will fail", filepath.Base(path))
		}
	}
}

// A file that did not exist before the mobile build must not be created by the restore.
// go.sum is absent in a fresh project until the first build, and writing an empty one
// would itself be a change.
func TestSnapshotModFilesDoesNotCreateFilesThatWereAbsent(t *testing.T) {
	dir := t.TempDir()
	gomod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(gomod, []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := snapshotModFiles(dir) // no go.sum present
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte("added by the build\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	restore()

	// The build's own go.sum is left alone rather than replaced with an empty file: there
	// was no previous content to restore, and truncating it would break the build that
	// just created it. Checking the CONTENT, not just existence — snapshotting an absent
	// file as nil bytes and writing that back leaves a zero-length file, which stats fine.
	got, err := os.ReadFile(filepath.Join(dir, "go.sum"))
	if err != nil {
		t.Fatalf("restore removed a go.sum it never snapshotted: %v", err)
	}
	if string(got) != "added by the build\n" {
		t.Errorf("restore overwrote a go.sum it never snapshotted with %q — an absent file "+
			"must not be 'restored' to empty", got)
	}
}
