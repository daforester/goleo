package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every feature in featureRegistry must be reachable by at least one Go scan pattern.
//
// This is the invariant, not the instance. A registry entry carries the Android
// permissions and the iOS usage descriptions a feature needs, but NOTHING consults it
// unless detectFeatureUsage names that feature first — and the two lists are edited
// independently. Add the registry entry, forget the pattern, and the feature is fully
// implemented, compiles, runs (nativeShellProviderTags forces the tag in for any provider
// the shells wire), and simply never gets its permissions declared.
//
// Microphone shipped exactly that way in 0.10.13: RegisterMicrophone() had no pattern, so
// `goleo build android` derived a release manifest with no RECORD_AUDIO and no
// MODIFY_AUDIO_SETTINGS. Nothing local showed it. `goleo emulate android` uses a STATIC dev
// manifest that lists both, so the mic demo worked on every emulator run; the gomobile bind
// linked runtime/microphone regardless; and the iOS Info.plist template hardcodes
// NSMicrophoneUsageDescription, so the one platform that was tested on hardware was fine.
// The first sign would have been a user's installed release build reporting "denied".
func TestEveryFeatureIsDetectable(t *testing.T) {
	detectable := map[string]bool{}
	for _, sp := range scanPatterns {
		// StringRef resolves its feature from the matched text rather than naming one,
		// so it cannot vouch for a specific feature here.
		if sp.Feature == "StringRef" {
			continue
		}
		detectable[sp.Feature] = true
	}
	for _, f := range featureRegistry {
		if !detectable[f.Name] {
			t.Errorf("featureRegistry has %q (%s) but no Go scan pattern names it — "+
				"detectFeatureUsage can never return %s, so its permissions and usage "+
				"descriptions are unreachable",
				f.Name, f.BuildTag, f.BuildTag)
		}
	}
}

// The same check the other way round: a scan pattern that names a feature the registry
// does not have is dead, and silently detects nothing (tagForName returns "").
func TestEveryScanPatternNamesARealFeature(t *testing.T) {
	for _, sp := range scanPatterns {
		if sp.Feature == "StringRef" {
			continue
		}
		if tagForName(sp.Feature) == "" {
			t.Errorf("scan pattern %q names feature %q, which is not in featureRegistry — "+
				"matches are discarded", sp.Pattern, sp.Feature)
		}
	}
}

// End-to-end over the REAL scaffold: extract the demo template and require the scanner to
// see every feature it registers.
//
// The two tests above compare two lists inside this package. This one compares the scanner's
// output against the thing a user actually runs it on, so it also catches a scaffold that
// starts registering a feature the registry has never heard of. `goleo new --demo` enables
// everything, which makes it the widest single input available — and it is the project the
// 2026-08-10 iOS run built, where `goleo_microphone` was absent from the printed feature list
// while `backend/app/app.go` called `RegisterMicrophone` twice.
func TestTheDemoScaffoldsFeaturesAreAllDetected(t *testing.T) {
	dir := t.TempDir()
	if err := extractDemoTemplate(dir, "scan-check"); err != nil {
		t.Fatal(err)
	}

	app, err := os.ReadFile(filepath.Join(dir, "backend", "app", "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	// The calls the scaffold really makes, minus commented-out ones — the same thing
	// detectFeatureUsage is looking at, read independently of scanPatterns so the test
	// cannot agree with the code by construction.
	registered := map[string]bool{}
	for _, m := range regexp.MustCompile(`runtime\.(Register\w+)\(`).
		FindAllStringSubmatch(stripGoLineComments(string(app)), -1) {
		registered[m[1]] = true
	}
	// Not permission-gated features, so they have no registry entry or build tag.
	for _, notAFeature := range []string{"RegisterBuiltins", "RegisterDesktopFeatures"} {
		delete(registered, notAFeature)
	}
	if len(registered) < 10 {
		t.Fatalf("only found %d Register* calls in the demo scaffold; the extraction or the "+
			"regexp is wrong, and a test that finds nothing passes vacuously", len(registered))
	}

	detected, err := detectFeatureUsage(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, tag := range detected {
		found[tag] = true
	}

	for call := range registered {
		// Map the call back to a tag through the same registry the permissions come from.
		// A call whose feature name is not derivable is reported rather than skipped.
		var tag string
		for _, f := range featureRegistry {
			short := strings.TrimPrefix(call, "Register")
			if strings.EqualFold(f.Name, short) ||
				// FileSystem/RegisterFS and Bluetooth/RegisterBLE do not share a name.
				(short == "FS" && f.Name == "FileSystem") ||
				(short == "BLE" && f.Name == "Bluetooth") {
				tag = f.BuildTag
				break
			}
		}
		if tag == "" {
			t.Errorf("the demo scaffold calls runtime.%s but featureRegistry has no feature "+
				"for it, so it can carry no permissions and no usage descriptions", call)
			continue
		}
		if !found[tag] {
			t.Errorf("the demo scaffold calls runtime.%s but the scanner did not detect %s. "+
				"Its Android permissions will be missing from every built manifest, and the "+
				"build's \"Detected mobile features\" line is the only place that shows it.\n"+
				"detected = %v", call, tag, detected)
		}
	}
}

// Drives the real scanner over a project that registers the microphone and requires the
// permissions to come out the far end, in the shape of
// TestResolveUsesTheVocabularyTheScannerEmits. Both audio permissions are load-bearing:
// see the Microphone entry in featureRegistry for why MODIFY_AUDIO_SETTINGS is the one
// whose absence fails silently.
func TestRegisteringTheMicrophoneResolvesTheAudioPermissions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "backend", "app")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	app := `package app

import "github.com/daforester/goleo/runtime"

func New() {
	a := runtime.New(runtime.Config{})
	runtime.RegisterMicrophone(a.Bridge())
}
`
	if err := os.WriteFile(filepath.Join(src, "app.go"), []byte(app), 0o644); err != nil {
		t.Fatal(err)
	}

	detected, err := detectFeatureUsage(dir)
	if err != nil {
		t.Fatal(err)
	}

	p := resolveAndroidPermissions(detected, nil)
	for _, want := range []string{
		"android.permission.RECORD_AUDIO",
		"android.permission.MODIFY_AUDIO_SETTINGS",
	} {
		if !hasPerm(p.Permissions, want) {
			t.Errorf("a project calling RegisterMicrophone did not resolve %s; "+
				"scanner emitted %v, permissions = %v", want, detected, p.Permissions)
		}
	}
	// RECORD_AUDIO implies android.hardware.microphone, which Play treats as required
	// unless it is declared explicitly — see TestEveryImpliedHardwareFeatureIsDeclaredOptional.
	if !hasPerm(p.Features, "android.hardware.microphone") {
		t.Errorf("microphone hardware feature missing; resolved features = %v", p.Features)
	}
}
