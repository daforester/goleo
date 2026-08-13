package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

// Go -> JS calls. The scripting layer's whole point: init.js defines functions, Go calls them.
func newScriptedRuntime(t *testing.T, script string) *JSRuntime {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backend", "init.js"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	jsr := NewJSRuntime(Config{DevMode: true, Title: "scripted", WindowMode: WindowModeBrowser}, nil)
	if err := jsr.Run(); err != nil {
		t.Fatalf("init script failed: %v", err)
	}
	t.Cleanup(jsr.Stop)
	return jsr
}

func TestJSCallRoundTrip(t *testing.T) {
	jsr := newScriptedRuntime(t, `
	  function double(n) { return n * 2; }
	  function greet(o) { return "hi " + o.name; }
	  function nothing() {}
	`)
	ctx := context.Background()

	if v, err := jsr.Call(ctx, "double", 21); err != nil || v != int64(42) {
		t.Errorf("double(21) = %v (%T), %v; want 42", v, v, err)
	}
	// A struct crosses as JSON — the boundary that keeps goja from silently dropping
	// fields it cannot map reflectively.
	if v, err := jsr.Call(ctx, "greet", struct {
		Name string `json:"name"`
	}{"ada"}); err != nil || v != "hi ada" {
		t.Errorf("greet = %v, %v; want \"hi ada\"", v, err)
	}
	if v, err := jsr.Call(ctx, "nothing"); err != nil || v != nil {
		t.Errorf("a function returning undefined should give (nil, nil), got %v, %v", v, err)
	}
}

func TestJSCallSurfacesErrorsAsErrors(t *testing.T) {
	jsr := newScriptedRuntime(t, `
	  function boom() { throw new Error("kaboom"); }
	  var notAFunction = 5;
	`)
	ctx := context.Background()

	// A JS throw must become a Go error, never a panic crossing the boundary.
	if _, err := jsr.Call(ctx, "boom"); err == nil || !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("a JS throw should surface as a Go error naming it, got: %v", err)
	}
	if _, err := jsr.Call(ctx, "missing"); err == nil || !strings.Contains(err.Error(), "not defined") {
		t.Errorf("calling an undefined function should say so, got: %v", err)
	}
	if _, err := jsr.Call(ctx, "notAFunction"); err == nil || !strings.Contains(err.Error(), "not a function") {
		t.Errorf("calling a non-function should say so, got: %v", err)
	}
	if !jsr.Has(ctx, "boom") || jsr.Has(ctx, "missing") || jsr.Has(ctx, "notAFunction") {
		t.Error("Has should report only callable globals")
	}
}

// THE reason this design exists. goja is not goroutine-safe and jsruntime.go had no
// locking, which was fine only while Run() was the sole toucher of the VM. Bridge handlers
// run one goroutine per request, so concurrent Call() is the normal case, not an edge one.
// Run this under -race; without the owning goroutine it is a data race, not a flake.
func TestJSCallIsSafeFromManyGoroutines(t *testing.T) {
	jsr := newScriptedRuntime(t, `
	  var hits = 0;
	  function bump() { hits = hits + 1; return hits; }
	`)
	ctx := context.Background()

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := jsr.Call(ctx, "bump"); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Call failed: %v", err)
	}
	// Serialised through one goroutine, so every increment lands.
	v, err := jsr.Call(ctx, "bump")
	if err != nil {
		t.Fatal(err)
	}
	if v != int64(n+1) {
		t.Errorf("hits = %v after %d concurrent calls; want %d — calls were lost or interleaved", v, n, n+1)
	}
}

// A runaway script would otherwise wedge the owning goroutine for the life of the process,
// taking every later call with it. Interrupt is the only way out.
func TestJSCallHonoursContextDeadline(t *testing.T) {
	jsr := newScriptedRuntime(t, `function spin() { while (true) {} }`)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := jsr.Call(ctx, "spin"); err == nil {
		t.Fatal("an infinite loop should have been interrupted by the deadline")
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("deadline took %v to fire — the VM was not interrupted", el)
	}
	// And the runtime must survive: the next caller gets a working VM, not a wedged one.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	if _, err := jsr.Call(ctx2, "spin"); err == nil {
		t.Error("expected the second call to be interrupted too")
	}
}

func TestJSCallAfterStopIsUnavailable(t *testing.T) {
	jsr := newScriptedRuntime(t, `function ok() { return 1; }`)
	ctx := context.Background()
	if _, err := jsr.Call(ctx, "ok"); err != nil {
		t.Fatal(err)
	}
	jsr.Stop()
	// Callers must get an error rather than blocking forever on a runtime going away.
	if _, err := jsr.Call(ctx, "ok"); !errors.Is(err, ErrJSUnavailable) {
		t.Errorf("after Stop, Call should return ErrJSUnavailable, got: %v", err)
	}
}

func TestJSCallJSONDecodesIntoAStruct(t *testing.T) {
	jsr := newScriptedRuntime(t, `function order() { return { id: "A1", total: 12.5 }; }`)
	var out struct {
		ID    string  `json:"id"`
		Total float64 `json:"total"`
	}
	if err := jsr.CallJSON(context.Background(), "order", &out); err != nil {
		t.Fatal(err)
	}
	if out.ID != "A1" || out.Total != 12.5 {
		t.Errorf("CallJSON gave %+v", out)
	}
}
