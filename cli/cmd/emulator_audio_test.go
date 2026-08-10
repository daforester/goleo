package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// The emulator "zeroes out audio" (its own -help text) unless started with
// -allow-host-audio, so a granted RECORD_AUDIO still yields no usable input device. goleo
// launched with only "-avd <name> -no-snapshot-load", which is why the microphone demo could
// be permitted and still find nothing to record from.
//
// Headless is the deliberate opposite: CI wants no audio at all, and -no-audio and
// -allow-host-audio contradict each other.
func TestEmulatorLaunchArgsPassHostAudio(t *testing.T) {
	cases := []struct {
		name           string
		headless       bool
		hostAudio      bool
		wantHostAudio  bool
		wantNoAudio    bool
		wantNoWindowed bool
	}{
		{name: "windowed with host audio support", hostAudio: true, wantHostAudio: true},
		{name: "windowed without host audio support"},
		{name: "headless never asks for host audio", headless: true, hostAudio: true,
			wantNoAudio: true, wantNoWindowed: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := emulatorLaunchArgs("goleo_avd", c.headless, c.hostAudio)

			// The parts that must never change.
			if !hasArg(args, "-avd") || !hasArg(args, "goleo_avd") {
				t.Errorf("the AVD is no longer passed: %v", args)
			}
			if !hasArg(args, "-no-snapshot-load") {
				t.Errorf("-no-snapshot-load was dropped: %v", args)
			}

			if got := hasArg(args, "-allow-host-audio"); got != c.wantHostAudio {
				t.Errorf("-allow-host-audio present = %v, want %v: %v",
					got, c.wantHostAudio, args)
			}
			if got := hasArg(args, "-no-audio"); got != c.wantNoAudio {
				t.Errorf("-no-audio present = %v, want %v: %v", got, c.wantNoAudio, args)
			}
			if got := hasArg(args, "-no-window"); got != c.wantNoWindowed {
				t.Errorf("-no-window present = %v, want %v: %v", got, c.wantNoWindowed, args)
			}

			// The two audio flags contradict each other; passing both would be a bug
			// whichever way the emulator resolved it.
			if hasArg(args, "-no-audio") && hasArg(args, "-allow-host-audio") {
				t.Errorf("both -no-audio and -allow-host-audio were passed: %v", args)
			}
		})
	}
}

// avdConfigValue replaced a single-key reader (avdSystemImageSysdir) so the microphone check
// could read hw.audioInput without a second config.ini parser. Both keys must work, and a
// missing key or file must read as "unknown" rather than as an empty value — callers treat
// those differently.
func TestAvdConfigValueReadsAnyKey(t *testing.T) {
	avdHome := t.TempDir()
	t.Setenv("ANDROID_AVD_HOME", avdHome)

	dir := filepath.Join(avdHome, "goleo_avd.avd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const cfg = "avd.ini.encoding=UTF-8\n" +
		"image.sysdir.1=system-images/android-34/google_apis/x86_64/\n" +
		"hw.audioInput=yes\n"
	if err := os.WriteFile(filepath.Join(dir, "config.ini"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := avdConfigValue("goleo_avd", "hw.audioInput"); got != "yes" {
		t.Errorf("hw.audioInput = %q, want %q", got, "yes")
	}
	// The original caller must keep working through the generalised reader.
	if got := avdSystemImageSysdir("goleo_avd"); !strings.HasPrefix(got, "system-images/") {
		t.Errorf("avdSystemImageSysdir = %q, want the image.sysdir.1 value", got)
	}
	if got := avdConfigValue("goleo_avd", "hw.notARealKey"); got != "" {
		t.Errorf("an absent key returned %q, want \"\"", got)
	}
	if got := avdConfigValue("no_such_avd", "hw.audioInput"); got != "" {
		t.Errorf("a missing AVD returned %q, want \"\"", got)
	}
}

// An AVD's data directory is NOT necessarily <name>.avd — it is whatever its <name>.ini
// registration points at, and the two diverge the moment an AVD is renamed. A real machine
// had emulator-5562.ini -> Medium_Phone.avd/, a perfectly healthy AVD that goleo could not
// introspect: avdStatus said "unable to verify its system image", ensureAVD's self-heal was
// silently skipped, and the microphone check advised editing a config.ini at a path that did
// not exist. Reading the .ini is what makes all three correct.
func TestAvdConfigFollowsTheIniPathWhenTheDirectoryIsNamedDifferently(t *testing.T) {
	avdHome := t.TempDir()
	t.Setenv("ANDROID_AVD_HOME", avdHome)

	// Data lives in Medium_Phone.avd, but the AVD is called emulator-5562.
	dataDir := filepath.Join(avdHome, "Medium_Phone.avd")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const cfg = "AvdId=emulator-5562\n" +
		"image.sysdir.1=system-images/android-36.1/google_apis_playstore/x86_64/\n" +
		"hw.audioInput=yes\n"
	if err := os.WriteFile(filepath.Join(dataDir, "config.ini"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	ini := "avd.ini.encoding=UTF-8\npath=" + dataDir + "\npath.rel=avd\\Medium_Phone.avd\n"
	if err := os.WriteFile(filepath.Join(avdHome, "emulator-5562.ini"), []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := avdConfigValue("emulator-5562", "hw.audioInput"); got != "yes" {
		t.Errorf("hw.audioInput = %q, want %q — the .ini's path= was not followed", got, "yes")
	}
	if got := avdSystemImageSysdir("emulator-5562"); !strings.Contains(got, "android-36.1") {
		t.Errorf("image.sysdir.1 = %q, want the value from the AVD's real directory", got)
	}

	// The ordinary case — directory named after the AVD, no .ini — must still work.
	writeAVDConfig(t, avdHome, "plain_avd", "system-images/android-34/google_apis/x86_64/")
	if got := avdSystemImageSysdir("plain_avd"); !strings.Contains(got, "android-34") {
		t.Errorf("the <name>.avd fallback broke: %q", got)
	}
}

// doctor reports microphone readiness but must never fail the run over it: a machine with no
// working microphone builds and emulates everything else perfectly well.
func TestAvdAudioStatusIsInformativeNotFatal(t *testing.T) {
	avdHome := t.TempDir()
	t.Setenv("ANDROID_AVD_HOME", avdHome)

	// No emulator resolved at all — the one case that must not claim to know anything.
	d := &androidDeps{}
	if got := d.avdAudioStatus(); !strings.Contains(got, "unknown") {
		t.Errorf("with no emulator, status = %q, want it to say unknown", got)
	}
}

// `emulator -list-avds` enumerates the *.ini files at the AVD home root, so it can name an
// AVD whose .avd directory does not exist. That is not hypothetical: a dev machine had a
// stray emulator-5562.ini listed while the only real AVD had no .ini at all. Reporting
// "add hw.audioInput=yes to its config.ini" there sends someone to a file that is not there,
// so a missing config must read differently from a missing key.
func TestAvdAudioStatusSeparatesMissingConfigFromMissingKey(t *testing.T) {
	avdHome := t.TempDir()
	t.Setenv("ANDROID_AVD_HOME", avdHome)
	emuPath := writeFakeEmulator(t, t.TempDir(), "phantom_avd")

	d := &androidDeps{EmulatorPath: emuPath}
	got := d.avdAudioStatus()
	if !strings.Contains(got, "no readable config.ini") {
		t.Errorf("an AVD with no .avd directory should say its config is unreadable, got %q", got)
	}
	if strings.Contains(got, "add hw.audioInput") {
		t.Errorf("advises editing a config.ini that does not exist: %q", got)
	}

	// And with a config present but the key absent, the advice IS appropriate.
	writeAVDConfig(t, avdHome, "phantom_avd", "system-images/android-34/google_apis/x86_64/")
	if got := d.avdAudioStatus(); !strings.Contains(got, "hw.audioInput") {
		t.Errorf("with a real config.ini lacking the key, status should name it: %q", got)
	}
}
