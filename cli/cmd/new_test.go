package cmd

import (
	"os"
	"path/filepath"
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
}
