package cmd

import (
	"fmt"
	"sort"
	"strings"
)

// Android manifest permissions, derived from what the app actually enables.
//
// The template used to declare thirteen permissions unconditionally — CAMERA,
// RECORD_AUDIO, both LOCATION grants, VIBRATE, NFC and four BLUETOOTH_* — for every
// app built with goleo, because the demo scaffold needed them. Play flags unjustified
// permissions, and a user installing a note-taking app should not be told it wants the
// camera and their location.
//
// DEVIATION worth stating plainly: the plan said to derive these from "the build tags
// actually compiled in" rather than from scan.go's source detection, on the grounds
// that a detection false negative would strip a permission and break a runtime request
// in the field. That reasoning is sound but the mechanism does not work here, because
// the compiled tag set is deliberately a SUPERSET: nativeShellProviderTags forces
// goleo_nfc, goleo_ble, goleo_sensors and five others into every single build so gobind
// emits the symbols the fixed Java shell references. Deriving from compiled tags would
// therefore still declare NFC, BLUETOOTH_SCAN/CONNECT and BODY_SENSORS for every app —
// it would not fix the thing it was meant to fix.
//
// So permissions follow ENABLEMENT (the Register* calls detectFeatureUsage finds), which
// is the only signal that reflects intent. The false-negative risk is real and is
// mitigated three ways rather than ignored:
//
//  1. mobile.android.extra_permissions is an explicit, documented override.
//  2. The build PRINTS every permission it declared and which feature asked for it, so
//     a missed detection is visible while the developer is building rather than after a
//     user installs and a runtime request silently returns "denied".
//  3. The core set below is unconditional, so the always-needed ones cannot be lost.
//
// If detection ever becomes unreliable enough that (2) is not enough, the answer is to
// make enablement explicit in goleo.json — not to declare everything.

// corePermissions are declared for every app.
//
// INTERNET because the whole architecture is a WebView talking to a loopback HTTP
// server; ACCESS_NETWORK_STATE because the bridge reports connectivity;
// POST_NOTIFICATIONS because notifications are a core builtin (RegisterBuiltins), not an
// opt-in feature, so it is not discoverable from a Register* call.
var corePermissions = []string{
	"android.permission.INTERNET",
	"android.permission.ACCESS_NETWORK_STATE",
	"android.permission.POST_NOTIFICATIONS",
}

// androidHardwareFeatures maps a feature build tag to the <uses-feature> declarations it
// needs. All are required="false": Play uses these to filter which devices can install
// an app, and goleo features degrade to a browser fallback rather than being essential,
// so marking any of them required would exclude devices unnecessarily.
//
// EVERY feature a declared permission IMPLIES has to be listed here, not just the obvious
// one per feature. aapt2 derives <uses-feature> entries from <uses-permission> and an
// implied entry defaults to required="TRUE" — so a permission whose feature is not declared
// explicitly becomes a hard device filter. Found on a real Play upload: the listing reported
// six features where goleo declared three, and the extra `android.hardware.bluetooth`
// (implied by BLUETOOTH/BLUETOOTH_ADMIN) and `android.hardware.location` (implied by
// ACCESS_FINE_LOCATION) were required, excluding any device without Bluetooth or GPS from an
// app that degrades gracefully on both. `android.hardware.faketouch` is also implied and is
// left alone: it is a baseline every device satisfies.
var androidHardwareFeatures = map[string][]string{
	"goleo_camera": {"android.hardware.camera"},
	"goleo_nfc":    {"android.hardware.nfc"},
	// RECORD_AUDIO implies android.hardware.microphone. Without this line an app that
	// registers the microphone would be filtered off any device without one — and the
	// recording it wants is a getUserMedia fallback that degrades fine.
	"goleo_microphone": {"android.hardware.microphone"},
	// bluetooth_le for the BLE APIs, and plain bluetooth because the legacy BLUETOOTH /
	// BLUETOOTH_ADMIN permissions imply it.
	"goleo_ble": {"android.hardware.bluetooth_le", "android.hardware.bluetooth"},
	// ACCESS_FINE_LOCATION implies android.hardware.location. The real Play listing reported
	// only that one, not .gps — but Android's documented table lists both and aapt's implied
	// set has changed across versions, so .gps is declared defensively. Declaring a feature
	// required="false" that nothing implies costs one inert line; missing one that IS implied
	// costs device coverage.
	"goleo_geolocation": {"android.hardware.location", "android.hardware.location.gps"},
}

// legacyBluetoothPermissions are the pre-API-31 Bluetooth grants. BLUETOOTH and
// BLUETOOTH_ADMIN were replaced by BLUETOOTH_SCAN/CONNECT in API 31, so they carry
// maxSdkVersion="30" — without that bound Play reports them as unnecessary on modern
// devices. They are emitted as raw XML because they need that attribute.
const legacyBluetoothXML = `    <uses-permission android:name="android.permission.BLUETOOTH" android:maxSdkVersion="30" />
    <uses-permission android:name="android.permission.BLUETOOTH_ADMIN" android:maxSdkVersion="30" />`

// androidManifestPerms is what the manifest template renders.
type androidManifestPerms struct {
	// Permissions are plain <uses-permission> names, sorted.
	Permissions []string
	// LegacyXML is pre-rendered XML for permissions needing extra attributes ("" if none).
	LegacyXML string
	// Features are <uses-feature> names, all required="false", sorted.
	Features []string
	// Explain maps each permission to the feature that asked for it, for the build log.
	Explain map[string]string
}

// resolveAndroidPermissions works out the manifest entries for a project.
//
// detectedTags is what detectFeatureUsage returns: BUILD TAGS ("goleo_camera"), not
// feature names ("Camera"). Getting that wrong is not hypothetical — the first version of
// this matched featureRegistry.Name against the tag list, so nothing ever matched and a
// demo project full of camera and Bluetooth pages resolved to three core permissions.
// The unit tests missed it because they passed "Camera" by hand: the value the code
// expected, and the one the producer never emits. TestResolveUsesTheVocabularyTheScanner
// EmitsNow feeds real scanner output through here so the two cannot drift apart again.
//
// extra comes from mobile.android.extra_permissions and is trusted verbatim, since it
// exists precisely for what detection cannot see.
func resolveAndroidPermissions(detectedTags []string, extra []string) androidManifestPerms {
	enabled := make(map[string]bool, len(detectedTags))
	for _, tag := range detectedTags {
		enabled[tag] = true
	}

	perms := map[string]string{} // permission -> what asked for it
	for _, p := range corePermissions {
		perms[p] = "core"
	}

	features := map[string]bool{}
	legacyBluetooth := false

	for _, f := range featureRegistry {
		if !enabled[f.BuildTag] {
			continue
		}
		for _, p := range f.Permissions {
			if _, seen := perms[p]; !seen {
				perms[p] = f.Name
			}
		}
		for _, hw := range androidHardwareFeatures[f.BuildTag] {
			features[hw] = true
		}
		if f.BuildTag == "goleo_ble" {
			legacyBluetooth = true
		}
	}

	for _, p := range extra {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Accept both the bare name and the fully qualified one, so goleo.json can say
		// "RECORD_AUDIO" or "android.permission.RECORD_AUDIO".
		if !strings.Contains(p, ".") {
			p = "android.permission." + p
		}
		perms[p] = "extra_permissions"
	}

	out := androidManifestPerms{Explain: perms}
	for p := range perms {
		out.Permissions = append(out.Permissions, p)
	}
	sort.Strings(out.Permissions)
	for f := range features {
		out.Features = append(out.Features, f)
	}
	sort.Strings(out.Features)
	if legacyBluetooth {
		out.LegacyXML = legacyBluetoothXML
	}
	return out
}

// reportAndroidPermissions prints what was declared and why.
//
// This is mitigation (2) from the file comment, and it is the reason deriving from
// enablement is safe enough: a developer who called RegisterGeolocation in a way the
// scanner missed sees ACCESS_FINE_LOCATION absent from this list at build time, instead
// of finding out when a user's location request silently returns denied.
func reportAndroidPermissions(p androidManifestPerms) {
	fmt.Printf("  Manifest permissions (%d):\n", len(p.Permissions))
	for _, name := range p.Permissions {
		short := strings.TrimPrefix(name, "android.permission.")
		fmt.Printf("    %-24s <- %s\n", short, p.Explain[name])
	}
	if len(p.Features) > 0 {
		fmt.Printf("  Hardware features (all optional): %s\n", strings.Join(p.Features, ", "))
	}
	fmt.Println("  If something you use is missing, the scanner did not see its Register* call —")
	fmt.Println("  add it to mobile.android.extra_permissions in goleo.json.")
}

// setAndroidPermissions resolves the manifest entries for cfg and reports them.
//
// Separate from loadMobileConfig because it needs to scan the project's source, which
// loadMobileConfig (a pure goleo.json reader used on every mobile path, including
// `goleo emulate`) has no business doing.
func setAndroidPermissions(cfg *mobileConfig, projectDir string) error {
	detected, err := detectFeatureUsage(projectDir)
	if err != nil {
		return fmt.Errorf("scanning for enabled features: %w", err)
	}
	cfg.Perms = resolveAndroidPermissions(detected, cfg.ExtraPermissions)
	reportAndroidPermissions(cfg.Perms)
	return nil
}
