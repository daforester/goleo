package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor [target]",
	Short: "Check build/emulate dependencies without installing or prompting for anything",
	Long: `Report whether the dependencies 'goleo build'/'goleo emulate' need are
already in place — discovery only, nothing is installed and nothing prompts.

Exists so scripts (CI, a project's own tooling wrapping goleo, an editor
extension) can check readiness without risking an interactive prompt that
would just hang with no attached terminal, and without hand-duplicating
goleo's own dependency-resolution order, which drifts out of sync with reality
the moment it changes upstream.

Targets:
  android    Java/JDK, gomobile, Android SDK, NDK, adb, emulator, and AVD
             status — the same set 'goleo build android'/'emulate android'
             resolve via ensureAndroidDeps().`,
	Args: cobra.ExactArgs(1),
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	switch args[0] {
	case "android":
		return doctorAndroid()
	default:
		return fmt.Errorf("unsupported target: %s. Supported: android", args[0])
	}
}

func doctorAndroid() error {
	fmt.Println("  Checking Android build dependencies (read-only)...")
	fmt.Println()

	deps, statuses := checkAndroidDeps()

	requiredMissing := false
	for _, s := range statuses {
		if s.err != nil {
			mark := "✗"
			if s.optional {
				mark = "○"
			} else {
				requiredMissing = true
			}
			fmt.Printf("  %s %-18s %v\n", mark, s.name, s.err)
			continue
		}
		fmt.Printf("  ✓ %-18s %s\n", s.name, s.path)
	}

	fmt.Printf("  %-20s %s\n", "AVD (emulate only):", deps.avdStatus())
	// Reported but never fatal: an environment with no working microphone builds
	// and runs everything else perfectly well.
	fmt.Printf("  %-20s %s\n", "Microphone:", deps.avdAudioStatus())
	fmt.Println()

	if requiredMissing {
		return fmt.Errorf("one or more required Android dependencies are missing — see above")
	}
	fmt.Println("  All required dependencies satisfied — build/emulate should not need to prompt.")
	return nil
}
