package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestChooseTemplate(t *testing.T) {
	// Only the flag-resolution branches are exercised here; the empty-flag case
	// prompts/reads stdin, which isn't safe to drive in a unit test.
	cases := []struct {
		tmpl string
		demo bool
		want string
		err  bool
	}{
		{tmpl: "demo", want: "demo"},
		{tmpl: "minimal", want: "minimal"},
		{tmpl: "DEMO", want: "demo"},
		{demo: true, want: "demo"},
		{tmpl: "bogus", err: true},
	}
	for _, c := range cases {
		newTemplate, newDemo = c.tmpl, c.demo
		got, err := chooseTemplate()
		if c.err {
			if err == nil {
				t.Errorf("chooseTemplate(%q,%v) expected error", c.tmpl, c.demo)
			}
			continue
		}
		if err != nil {
			t.Errorf("chooseTemplate(%q,%v) unexpected error: %v", c.tmpl, c.demo, err)
		}
		if got != c.want {
			t.Errorf("chooseTemplate(%q,%v) = %q, want %q", c.tmpl, c.demo, got, c.want)
		}
	}
	newTemplate, newDemo = "", false
}

func TestExtractDemoTemplate(t *testing.T) {
	dir := t.TempDir()
	if err := extractDemoTemplate(dir, "myapp"); err != nil {
		t.Fatal(err)
	}

	// Verbatim file with Vue braces must survive untouched.
	for _, f := range []string{
		"frontend/src/App.vue",
		"frontend/src/demos/BatteryDemo.vue",
		"frontend/src/demos/registry.ts",
		"backend/commands/commands.go", // .tmpl stripped
		"backend/app/app.go",           // .tmpl stripped
		"go.mod",                       // .tmpl stripped
		".gitignore",                   // gitignore → .gitignore
	} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}

	// No .tmpl artifacts should remain, and the name token must be substituted.
	if _, err := os.Stat(filepath.Join(dir, "go.mod.tmpl")); err == nil {
		t.Error("go.mod.tmpl was not renamed")
	}
	goMod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(goMod), "module goleo/myapp") {
		t.Errorf("go.mod module not substituted:\n%s", goMod)
	}
	appGo, _ := os.ReadFile(filepath.Join(dir, "backend/app/app.go"))
	if strings.Contains(string(goMod)+string(appGo), demoAppNameToken) {
		t.Error("name token left unreplaced")
	}
	// The version token must be substituted too — a leftover would make the
	// generated go.mod unparseable, and a hardcoded version would go stale.
	if strings.Contains(string(goMod), demoVersionToken) {
		t.Errorf("version token left unreplaced:\n%s", goMod)
	}
	if want := "require github.com/daforester/goleo " + scaffoldGoleoVersion(); !strings.Contains(string(goMod), want) {
		t.Errorf("go.mod should require %q, got:\n%s", want, goMod)
	}
}

// The scaffolded require must track the CLI's own version rather than a
// hardcoded literal (which had gone stale by dozens of releases at v0.2.1).
func TestScaffoldGoleoVersionTracksCLI(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	Version = "1.2.3"
	if got := scaffoldGoleoVersion(); got != "v1.2.3" {
		t.Errorf("stamped build: got %q, want v1.2.3", got)
	}

	// An unstamped dev build has no published version to name, so it must fall
	// back to the placeholder — which fails loudly if resolution never happens,
	// instead of silently resolving to a real but ancient release.
	//
	// "dev" makes resolveVersion consult debug.ReadBuildInfo(), which under `go
	// test` reports a pseudo-version of an unpushed commit (often with a +dirty
	// suffix). Pinning that would bake an unfetchable commit into every scaffold,
	// so it must be rejected too — this is why scaffoldGoleoVersion uses the
	// exact-match releaseVersionRe rather than semverRe's prefix match.
	Version = "dev"
	if got := scaffoldGoleoVersion(); got != scaffoldPlaceholderVersion {
		t.Errorf("dev build: got %q, want %q", got, scaffoldPlaceholderVersion)
	}
}

// Guards the semverRe-vs-releaseVersionRe distinction directly: a pseudo-version
// must never be pinned into a scaffold even though it *starts* with a valid
// X.Y.Z, because the commit it names may not be pushed anywhere.
func TestReleaseVersionReRejectsPseudoVersions(t *testing.T) {
	for _, v := range []string{"0.8.7", "1.0.0", "10.20.30"} {
		if !releaseVersionRe.MatchString(v) {
			t.Errorf("%q is a published release and should be pinnable", v)
		}
	}
	for _, v := range []string{
		"dev",
		"0.8.8-0.20260803161633-d8da50055a4e",
		"0.8.8-0.20260803161633-d8da50055a4e+dirty",
		"0.8.7+dirty",
		"0.9.0-rc1",
		"",
	} {
		if releaseVersionRe.MatchString(v) {
			t.Errorf("%q is not a clean published release and must not be pinned", v)
		}
		// semverRe (used by ensureGoleoRequire, which self-heals via @latest) is
		// intentionally looser; assert the two really do differ so a future
		// refactor doesn't silently collapse them back together.
		if v == "0.8.8-0.20260803161633-d8da50055a4e" && !semverRe.MatchString(v) {
			t.Error("expected semverRe to be the looser prefix match")
		}
	}
}

// Both scaffold templates (minimal via text/template, demo via token
// replacement) must agree on the version, and neither may carry a literal
// pinned version any more.
func TestScaffoldTemplatesCarryNoHardcodedVersion(t *testing.T) {
	rendered, err := renderTemplate(tmplGoMod, projectConfig{
		Name: "x", ModuleName: "goleo/x", GoleoVersion: "v9.9.9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "require github.com/daforester/goleo v9.9.9") {
		t.Errorf("minimal template did not take the injected version:\n%s", rendered)
	}

	demoTmpl, err := mobileTemplates.ReadFile("templates/demo/go.mod.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(demoTmpl), demoVersionToken) {
		t.Errorf("demo go.mod.tmpl should use %s, got:\n%s", demoVersionToken, demoTmpl)
	}
	// Guard against a literal `goleo vX.Y.Z` creeping back into either template.
	hardcoded := regexp.MustCompile(`github\.com/daforester/goleo\s+v\d+\.\d+\.\d+`)
	for name, body := range map[string]string{
		"tmplGoMod":                  tmplGoMod,
		"templates/demo/go.mod.tmpl": string(demoTmpl),
	} {
		if m := hardcoded.FindString(body); m != "" {
			t.Errorf("%s hardcodes a goleo version (%q); it must be injected instead", name, m)
		}
	}
}
