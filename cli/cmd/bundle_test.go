package cmd

import (
	"path/filepath"
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

	yaml := nfpmConfig(cfg, "/tmp/app", "app")
	for _, want := range []string{"name: \"my-app\"", "version: \"1.2.3\"", "Acme", "/usr/bin/app"} {
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
