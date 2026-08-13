package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeFS map[string]string

func (f fakeFS) ReadFile(name string) ([]byte, error) {
	if c, ok := f[name]; ok {
		return []byte(c), nil
	}
	return nil, os.ErrNotExist
}

func TestInitScriptExplicitMissingErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	jsr := NewJSRuntime(Config{DevMode: true, InitJS: "nope.js", WindowMode: WindowModeBrowser}, nil)
	if err := jsr.Run(); err == nil {
		t.Fatal("expected error when explicitly configured init script is missing")
	}
}

func TestInitScriptDefaultMissingFallsBack(t *testing.T) {
	t.Chdir(t.TempDir())
	jsr := NewJSRuntime(Config{DevMode: true, WindowMode: WindowModeBrowser}, nil)
	if err := jsr.Run(); err != nil {
		t.Fatalf("expected silent fallback when no default init script exists, got: %v", err)
	}
}

func TestInitScriptDefaultFromDisk(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	script := `console.log("starting", getConfig().title)`
	if err := os.WriteFile(filepath.Join(dir, "backend", "init.js"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	jsr := NewJSRuntime(Config{DevMode: true, Title: "TestApp", WindowMode: WindowModeBrowser}, nil)
	if err := jsr.Run(); err != nil {
		t.Fatalf("expected backend/init.js to be found and executed, got: %v", err)
	}
}

func TestInitScriptExplicitFromDisk(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "custom.js"), []byte(`getConfig()`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	jsr := NewJSRuntime(Config{DevMode: true, InitJS: "custom.js", WindowMode: WindowModeBrowser}, nil)
	if err := jsr.Run(); err != nil {
		t.Fatalf("expected explicit init script to load, got: %v", err)
	}
}

func TestInitScriptEmbedded(t *testing.T) {
	jsr := NewJSRuntime(Config{
		EmbedFS:    fakeFS{"init.js": `createWindow({ title: "x" })`},
		WindowMode: WindowModeBrowser,
	}, nil)
	if err := jsr.Run(); err != nil {
		t.Fatalf("expected embedded init.js to load, got: %v", err)
	}
	if jsr.win != nil {
		t.Fatal("browser mode must not create a native window")
	}
}

func TestInitScriptSyntaxErrorReported(t *testing.T) {
	jsr := NewJSRuntime(Config{
		EmbedFS:    fakeFS{"init.js": `this is not javascript`},
		WindowMode: WindowModeBrowser,
	}, nil)
	if err := jsr.Run(); err == nil {
		t.Fatal("expected error for invalid init script")
	}
}

// The scaffolds' init.js documented an API the engine does not have.
//
// Both shipped ~35 lines describing `bridge.invoke("goleo:...")` and a catalogue of
// goleo:* commands, while provideAPI defines exactly three globals. There is no bridge
// object, so every documented call failed with a ReferenceError, and the block also
// listed goleo:geolocationGetCurrentPosition, removed in 0.11.0. A developer following
// it lost their first hour to documentation we wrote.
//
// This is the repo's recurring shape inverted — usually a declaration nothing consumes,
// here documentation for a declaration never made — so it is guarded rather than just
// corrected. The probe runs through the real Run() path: a false claim becomes a script
// error rather than a silent mismatch.
func TestVMDefinesExactlyTheDocumentedGlobals(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	probe := `
	  var missing = [];
	  ["getConfig", "createWindow", "console"].forEach(function (n) {
	    if (typeof this[n] === "undefined") { missing.push(n); }
	  }, this);
	  if (missing.length) { throw new Error("documented but absent: " + missing.join(", ")); }

	  // Documented as NOT existing. If one of these ever appears, the scaffold comment
	  // block is now understating the API and must be updated in the same change.
	  var unexpected = [];
	  ["bridge", "emit", "on", "setMenu", "quit", "invoke"].forEach(function (n) {
	    if (typeof this[n] !== "undefined") { unexpected.push(n); }
	  }, this);
	  if (unexpected.length) { throw new Error("exists but undocumented: " + unexpected.join(", ")); }
	`
	if err := os.WriteFile(filepath.Join(dir, "backend", "init.js"), []byte(probe), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	jsr := NewJSRuntime(Config{DevMode: true, Title: "probe", WindowMode: WindowModeBrowser}, nil)
	if err := jsr.Run(); err != nil {
		t.Fatalf("the JS engine's globals no longer match what the scaffolds document: %v", err)
	}
}

// And the scaffolds themselves must not re-grow the fabricated section. Checked as text
// because that comment block IS the documentation — there is no generated .d.ts for
// init.js yet, so a developer has nothing else to read.
func TestScaffoldInitJSDoesNotClaimABridge(t *testing.T) {
	for _, src := range []string{
		filepath.Join("..", "cli", "cmd", "templates.go"),
		filepath.Join("..", "cli", "cmd", "templates", "demo", "backend", "init.js"),
	} {
		raw, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		body := string(raw)
		if i := strings.Index(body, "var tmplInitJS"); i >= 0 {
			end := strings.Index(body[i:], "\n`\n")
			if end > 0 {
				body = body[i : i+end]
			}
		}
		for _, claim := range []string{"bridge.invoke", "goleo:getOS", "goleo:fsReadTextFile", "goleo:geolocationGetCurrentPosition"} {
			if strings.Contains(body, claim) {
				t.Errorf("%s: init.js documentation claims %q, which cannot be called from init.js — "+
					"the engine defines only getConfig, createWindow and console", src, claim)
			}
		}
		for _, real := range []string{"getConfig()", "createWindow(opts)", "console.log"} {
			if !strings.Contains(body, real) {
				t.Errorf("%s: init.js documentation no longer mentions %s, which the engine does define", src, real)
			}
		}
	}
}
