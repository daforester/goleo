package cmd

import "testing"

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
