package cmd

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"My App":        "my-app",
		"Goleo App!!":   "goleo-app",
		"  Spaced  Out": "spaced-out",
		"already-slug":  "already-slug",
		"A/B\\C":        "a-b-c",
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGeneratedArtifacts(t *testing.T) {
	cfg := bundleConfig{AppName: "My App", Version: "1.2.3", Identifier: "com.example.app", Publisher: "Acme <a@b.c>"}

	plist := infoPlist(cfg, "app")
	for _, want := range []string{"com.example.app", "1.2.3", "<string>app</string>", "CFBundleShortVersionString"} {
		if !contains(plist, want) {
			t.Errorf("Info.plist missing %q:\n%s", want, plist)
		}
	}

	nsi := nsisScript(cfg, `C:\out\app.exe`, "app.exe", `C:\out\setup.exe`)
	for _, want := range []string{`Name "My App"`, "OutFile", "WriteUninstaller", "Uninstall"} {
		if !contains(nsi, want) {
			t.Errorf("NSIS script missing %q:\n%s", want, nsi)
		}
	}

	yaml := nfpmConfig(cfg, "/tmp/app", "app", "", "/tmp/my-app.desktop", "amd64")
	for _, want := range []string{"name: \"my-app\"", "version: \"1.2.3\"", "Acme", "/usr/bin/app", "my-app.desktop"} {
		if !contains(yaml, want) {
			t.Errorf("nfpm config missing %q:\n%s", want, yaml)
		}
	}
}

func TestBinaryOutputName(t *testing.T) {
	win := buildTarget{OutputExt: ".exe"}
	nix := buildTarget{OutputExt: ""}
	cases := []struct {
		name, o string
		target  buildTarget
		want    string
	}{
		{"default windows", "", win, "app.exe"},
		{"default linux", "", nix, "app"},
		{"custom windows", "myapp", win, "myapp.exe"},
		{"custom already has ext", "myapp.exe", win, "myapp.exe"}, // no doubling
		{"custom linux", "myapp", nix, "myapp"},
	}
	for _, c := range cases {
		buildOutput = c.o
		if got := binaryOutputName(c.target); got != c.want {
			t.Errorf("%s: binaryOutputName(-o=%q) = %q, want %q", c.name, c.o, got, c.want)
		}
	}
	buildOutput = ""
}

func TestInstallerName(t *testing.T) {
	cfg := bundleConfig{AppName: "My App", Version: "1.2.3"}

	buildOutput = ""
	if got := installerName(cfg, ".exe", "-setup"); got != "my-app-1.2.3-setup.exe" {
		t.Errorf("default installer name = %q, want my-app-1.2.3-setup.exe", got)
	}
	if got := installerName(cfg, ".dmg", ""); got != "my-app-1.2.3.dmg" {
		t.Errorf("default dmg name = %q, want my-app-1.2.3.dmg", got)
	}

	buildOutput = "cool"
	if got := installerName(cfg, ".exe", "-setup"); got != "cool-setup.exe" {
		t.Errorf("-o installer name = %q, want cool-setup.exe", got)
	}
	buildOutput = filepath.Join("out", "cool.exe") // path + ext -> base name only
	if got := installerName(cfg, ".exe", "-setup"); got != "cool-setup.exe" {
		t.Errorf("-o path installer name = %q, want cool-setup.exe", got)
	}
	buildOutput = ""
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// The test above builds synthetic buildTargets, which is why it never caught the
// real bug: the `targets` MAP had "current" hardcoded to OutputExt: "" while taking
// the host's GOOS. So `goleo build` on Windows — the default command — wrote a
// binary named `app` with no `.exe`, which Windows refuses to execute (double-click
// does nothing, Start-Process errors). Only the explicit `goleo build windows`
// cross-target was right. Assert the actual table, not a stand-in.
func TestTargetsTableExtensionsMatchTheirGOOS(t *testing.T) {
	for name, target := range targets {
		switch name {
		case "android", "ios", "pwa":
			continue // not plain executables; their own extensions are asserted elsewhere
		}
		want := desktopOutputExt(target.GOOS)
		if target.OutputExt != want {
			t.Errorf("targets[%q] has GOOS=%q but OutputExt=%q; want %q — "+
				"a Windows build without .exe is not executable",
				name, target.GOOS, target.OutputExt, want)
		}
	}

	// Pin the two that matter most, so a refactor of desktopOutputExt cannot make
	// the loop above vacuously true.
	if got := targets["windows"].OutputExt; got != ".exe" {
		t.Errorf(`targets["windows"].OutputExt = %q, want ".exe"`, got)
	}
	if runtime.GOOS == "windows" {
		if got := targets["current"].OutputExt; got != ".exe" {
			t.Errorf(`on Windows, targets["current"].OutputExt = %q, want ".exe"`, got)
		}
	} else if got := targets["current"].OutputExt; got != "" {
		t.Errorf(`on %s, targets["current"].OutputExt = %q, want ""`, runtime.GOOS, got)
	}
}

// The whole point is the file name a user ends up with, so check that too — the
// layer the bug actually surfaced at.
func TestCurrentTargetProducesARunnableNameOnWindows(t *testing.T) {
	buildOutput = ""
	t.Cleanup(func() { buildOutput = "" })

	got := binaryOutputName(targets["current"])
	if runtime.GOOS == "windows" {
		if got != "app.exe" {
			t.Errorf("`goleo build` on Windows names the binary %q; Windows cannot run that", got)
		}
	} else if got != "app" {
		t.Errorf("`goleo build` on %s names the binary %q, want \"app\"", runtime.GOOS, got)
	}
}

// The installer is the second, worse consequence of the missing .exe, and nothing
// caught it: TestGeneratedArtifacts calls nsisScript with a hardcoded "app.exe"
// while production passed filepath.Base of the built binary — which for the
// `current` target on Windows was `app`. So `goleo build --bundle` (what the
// goleo:bundle script runs) produced an installer that installed a file with no
// extension and a Start Menu shortcut pointing at it. It installs fine and the
// shortcut does nothing.
//
// Derive the name the way bundleWindows does instead of restating it.
func TestWindowsInstallerReferencesAnExecutable(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the current target only yields a Windows installer on Windows")
	}
	buildOutput = ""
	t.Cleanup(func() { buildOutput = "" })

	binaryPath, err := filepath.Abs(binaryOutputName(targets["current"]))
	if err != nil {
		t.Fatal(err)
	}
	binBase := filepath.Base(binaryPath)
	if filepath.Ext(binBase) != ".exe" {
		t.Fatalf("bundling would install %q, which Windows cannot execute", binBase)
	}

	cfg := bundleConfig{AppName: "My App", Version: "1.2.3", Identifier: "com.example.app"}
	nsi := nsisScript(cfg, binaryPath, binBase, `C:\out\setup.exe`)

	// The shortcut is the bit a user clicks; it must target the executable.
	if !contains(nsi, `CreateShortcut "$SMPROGRAMS\My App.lnk" "$INSTDIR\app.exe"`) {
		t.Errorf("NSIS shortcut does not point at app.exe:\n%s", nsi)
	}
	// And the uninstaller must delete the same name it installed.
	if !contains(nsi, `Delete "$INSTDIR\app.exe"`) {
		t.Errorf("NSIS uninstall does not remove app.exe:\n%s", nsi)
	}
}

// nfpm's arch was hardcoded to "amd64", so `goleo build linux --arch arm64 --bundle`
// produced a package containing an arm64 binary but LABELLED amd64. dpkg refuses that
// on an arm64 machine, and an amd64 machine installs a binary it cannot run — a
// mislabelled package is worse than a missing one, because it fails at first launch
// rather than at install time.
func TestNfpmArchFollowsTheTarget(t *testing.T) {
	cfg := bundleConfig{AppName: "My App", Version: "1.2.3"}

	for _, goarch := range []string{"amd64", "arm64", "386", "arm"} {
		yaml := nfpmConfig(cfg, "/tmp/app", "app", "", "", goarch)
		want := "arch: \"" + goarch + "\""
		if !contains(yaml, want) {
			t.Errorf("goarch %q produced no %q:\n%s", goarch, want, yaml)
		}
		// And must not still carry the hardcoded value when targeting something else.
		if goarch != "amd64" && contains(yaml, `arch: "amd64"`) {
			t.Errorf("goarch %q still emitted arch: \"amd64\"", goarch)
		}
	}

	// An empty arch must not emit an empty field — nfpm would reject or mislabel it.
	if got := nfpmArch(""); got != "amd64" {
		t.Errorf("nfpmArch(\"\") = %q, want a concrete default", got)
	}
}

// The wrapper jar and the distribution the template requests must be the same Gradle
// version. They had drifted — jar from v8.10.2, distributionUrl asking for 9.4.1 — the
// kind of mismatch that works until it doesn't, and then fails inside Gradle's own
// bootstrap where the cause is invisible.
func TestGradleWrapperJarMatchesTheTemplateDistribution(t *testing.T) {
	props, err := mobileTemplates.ReadFile("templates/android/gradle/wrapper/gradle-wrapper.properties")
	if err != nil {
		t.Fatal(err)
	}
	want := "gradle-" + gradleWrapperVersion + "-bin.zip"
	if !contains(string(props), want) {
		t.Errorf("gradle-wrapper.properties does not request %s (gradleWrapperVersion=%s):\n%s",
			want, gradleWrapperVersion, props)
	}
	// Guard against a literal version creeping back into the download URL.
	if contains(gradleWrapperJarURL(), "8.10.2") && gradleWrapperVersion != "8.10.2" {
		t.Errorf("the wrapper jar URL is pinned to 8.10.2 while the template wants %s", gradleWrapperVersion)
	}
	if !contains(gradleWrapperJarURL(), gradleWrapperVersion) {
		t.Errorf("the wrapper jar URL %q does not reference %s", gradleWrapperJarURL(), gradleWrapperVersion)
	}
}
