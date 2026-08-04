package cmd

import (
	"io"
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

// The npm side of the same rule. This is the bug the Go-side test above was
// written for, still live one line away: tmplFrontendPackageJSON hardcoded
// "@goleo/bridge": "^0.2.1" long after the Go require started tracking releases.
//
// A caret on a 0.x version locks the MINOR, so ^0.2.1 resolves to 0.2.9 — not
// 0.8.x. Every scaffolded project therefore paired a v0.8.8 Go runtime with a
// bridge six minors old, across the wire contract the two sides must agree on
// (writeBinaryFile sent TextDecoder output where the runtime expects base64, so
// binary writes were simply broken in new projects).
func TestScaffoldTemplatesCarryNoHardcodedBridgeVersion(t *testing.T) {
	rendered, err := renderTemplate(tmplFrontendPackageJSON, projectConfig{
		Name: "x", ModuleName: "goleo/x", GoleoVersion: "v9.9.9", BridgeVersion: "^9.9.9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, `"@goleo/bridge": "^9.9.9"`) {
		t.Errorf("minimal frontend template did not take the injected bridge version:\n%s", rendered)
	}

	demoPkg, err := mobileTemplates.ReadFile("templates/demo/frontend/package.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(demoPkg), demoBridgeVersionToken) {
		t.Errorf("demo frontend/package.json should use %s, got:\n%s", demoBridgeVersionToken, demoPkg)
	}

	// Catch the class, not just this instance: no scaffold template may pin any
	// @goleo/* package to a literal version.
	hardcoded := regexp.MustCompile(`"@goleo/[a-z-]+":\s*"[~^]?\d+\.\d+\.\d+"`)
	for name, body := range map[string]string{
		"tmplFrontendPackageJSON":              tmplFrontendPackageJSON,
		"templates/demo/frontend/package.json": string(demoPkg),
	} {
		if m := hardcoded.FindString(body); m != "" {
			t.Errorf("%s hardcodes an @goleo/* version (%s); it must be injected instead", name, m)
		}
	}
}

// The two pins must resolve to the SAME release. Skew between them is the actual
// failure mode: the Go runtime and the bridge implement two halves of one wire
// contract, so a project must not get 0.8.8 of one and 0.2.9 of the other.
func TestScaffoldGoAndBridgeVersionsAgree(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	Version = "1.2.3"
	if got, want := scaffoldGoleoVersion(), "v1.2.3"; got != want {
		t.Errorf("scaffoldGoleoVersion() = %q, want %q", got, want)
	}
	if got, want := scaffoldBridgeVersion(), "^1.2.3"; got != want {
		t.Errorf("scaffoldBridgeVersion() = %q, want %q", got, want)
	}

	// A dev build has no release to match. The Go side uses the v0.0.0 placeholder
	// that a local replace can satisfy; npm has no equivalent, so `latest` is the
	// honest answer (and `goleo new` npm-links the local bridge anyway).
	Version = "dev"
	if got := scaffoldBridgeVersion(); got != "latest" && !strings.HasPrefix(got, "^") {
		t.Errorf("dev scaffoldBridgeVersion() = %q, want latest or a caret range", got)
	}
}

// The glaze fork replace is written out in three places — the root go.mod and both
// scaffold templates — and only a manual step in AGENTS.md's rebase instructions
// keeps them together. A downstream project that inherits a stale pin silently
// loses the Windows camera/mic/geolocation grant (the sole reason the fork exists),
// and because Go replace directives don't transit, nothing else would flag it.
//
// The root go.mod is the source of truth here: it is what CI vendors against.
func TestScaffoldTemplatesPinTheSameGlazeFork(t *testing.T) {
	rootMod, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`replace\s+github\.com/crgimenes/glaze\s+=>\s+(\S+\s+\S+)`)
	rootMatch := re.FindSubmatch(rootMod)
	if rootMatch == nil {
		t.Fatal("no glaze replace in the root go.mod — did the fork get dropped? " +
			"if so this test and both scaffold templates need updating too")
	}
	want := string(rootMatch[1])

	demoMod, err := mobileTemplates.ReadFile("templates/demo/go.mod.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"tmplGoMod":                  tmplGoMod,
		"templates/demo/go.mod.tmpl": string(demoMod),
	} {
		m := re.FindStringSubmatch(body)
		if m == nil {
			t.Errorf("%s has no glaze replace; scaffolded projects need it to inherit "+
				"the Windows permission grant (replace directives don't transit)", name)
			continue
		}
		if m[1] != want {
			t.Errorf("%s pins glaze at %q but the root go.mod uses %q — "+
				"scaffolded projects would get a different fork build", name, m[1], want)
		}
	}
}

// warnStaleBridgePin must fire on the ranges that genuinely cannot resolve, and
// stay quiet on everything a developer chose deliberately. The quiet cases matter
// more than the loud one: a warning printed on every `goleo dev` for a perfectly
// good pin trains people to ignore it.
func TestMinorOfClassifiesNpmRanges(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"^0.8.8", "0.8"},
		{"~0.8.8", "0.8"},
		{"0.8.8", "0.8"},
		{"v1.2.3", "1.2"},
		{"^0.2.1", "0.2"}, // the stale pin
		// Deliberate or non-comparable: must yield "" so no warning is emitted.
		{"latest", ""},
		{"*", ""},
		{"0.x", ""},
		{"^0", ""},
		{">=0.8.0 <2.0.0", ""},
		{"file:../bridge", ""},
		{"github:daforester/goleo", ""},
		{"", ""},
	} {
		if got := minorOf(tc.in); got != tc.want {
			t.Errorf("minorOf(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWarnStaleBridgePinOnlyForIncompatibleMinors(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })
	Version = "0.8.8"

	write := func(t *testing.T, pin string) string {
		t.Helper()
		dir := t.TempDir()
		fe := filepath.Join(dir, "frontend")
		if err := os.MkdirAll(fe, 0755); err != nil {
			t.Fatal(err)
		}
		body := `{"dependencies":{"@goleo/bridge":"` + pin + `","vue":"^3.4.0"}}`
		if err := os.WriteFile(filepath.Join(fe, "package.json"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	capture := func(t *testing.T, dir string) string {
		t.Helper()
		old := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout = w
		warnStaleBridgePin(dir)
		w.Close()
		os.Stdout = old
		out, _ := io.ReadAll(r)
		return string(out)
	}

	// The real stale pin: 0.2 cannot reach 0.8, so warn — and name the fix.
	out := capture(t, write(t, "^0.2.1"))
	if !strings.Contains(out, "@goleo/bridge") || !strings.Contains(out, "0.8.8") {
		t.Errorf("expected a warning naming the correct version, got:\n%s", out)
	}
	if !strings.Contains(out, "npm install @goleo/bridge@0.8.8") {
		t.Errorf("warning should give the exact command, got:\n%s", out)
	}

	for _, quiet := range []string{"^0.8.8", "^0.8.0", "0.8.8", "latest", "*", "file:../bridge"} {
		if out := capture(t, write(t, quiet)); out != "" {
			t.Errorf("pin %q should not warn, got:\n%s", quiet, out)
		}
	}

	// No frontend at all (a PWA-less or unusual layout) must not warn.
	if out := capture(t, t.TempDir()); out != "" {
		t.Errorf("missing frontend/package.json should not warn, got:\n%s", out)
	}

	// A dev build of the CLI has no authoritative version to compare against.
	Version = "dev"
	if out := capture(t, write(t, "^0.2.1")); out != "" {
		t.Errorf("dev CLI should not warn about pins, got:\n%s", out)
	}
}
