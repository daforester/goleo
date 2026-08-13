package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
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
	// Enumerate what goleo ADDS, by diffing against a vanilla goja runtime rather than
	// checking a hand-written list. A fixed list only catches globals someone thought to
	// name; this catches every one, including the next one added — which is the failure
	// mode that let `goleo` appear with the docs still saying no such object existed.
	// Wrapped in an IIFE so the probe's own vars do not become globals and pollute the
	// very set it is measuring — they did on the first attempt, which is a neat miniature
	// of why this test enumerates instead of trusting a list.
	probe := `
	  (function () {
	    var known = {};
	    ` + vanillaGlobalsJSON(t) + `.forEach(function (n) { known[n] = true; });
	    var added = Object.getOwnPropertyNames(globalThis).filter(function (n) { return !known[n]; });
	    added.sort();
	    var want = ["console", "createWindow", "getConfig", "goleo"];
	    if (added.join(",") !== want.join(",")) {
	      throw new Error("globals are [" + added.join(",") + "] but the docs describe [" + want.join(",") + "]");
	    }
	  })();
	`
	if err := os.WriteFile(filepath.Join(dir, "backend", "init.js"), []byte(probe), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	// Built with a real App, because that is what a real project has: provideBridgeAPI
	// installs `goleo` only when there is a bridge behind it, so a runtime constructed with
	// a nil app legitimately has three globals rather than four. Testing the nil case here
	// would assert the shape no shipped app ever sees.
	app := New(Config{DevMode: true, Title: "probe", WindowMode: WindowModeBrowser})
	app.jsr = NewJSRuntime(app.config, app)
	t.Cleanup(app.jsr.Stop)
	if err := app.jsr.Run(); err != nil {
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

// Third face of the same contract: the generated init.d.ts. The VM, the scaffold comment
// block and this file all describe the same three globals, and any two of them agreeing
// while the third drifts is how the fabricated `bridge` survived. Editors read the .d.ts,
// so it is the one a developer actually feels.
func TestGeneratedInitDTSMatchesTheVM(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "cli", "cmd", "generate.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	// Anchored on initDTSHeader, the template the generator renders. It was `initDTS` until
	// the overloads became generated from KnownCommands; this test broke on the rename, which
	// is the right kind of breakage — it means the anchor is load-bearing rather than decorative.
	i := strings.Index(body, "const initDTSHeader = ")
	if i < 0 {
		t.Fatal("initDTSHeader is gone from generate.go — backend/init.js would have no types again")
	}
	dts := body[i:]

	// The per-command overloads are generated, not literal, so assert the seam exists rather
	// than looking for a specific command in the template.
	if !strings.Contains(dts, "__GOLEO_INVOKE_OVERLOADS__") {
		t.Error("the invoke-overload placeholder is gone, so goleo.invoke would lose its " +
			"per-command typing and every mistyped command name would type-check again")
	}
	// A catch-all taking a plain string matches every typo and makes the overloads
	// decorative. It was there in the first cut and silently defeated the whole point.
	if strings.Contains(dts, "invoke(method: string") {
		t.Error("init.d.ts declares a catch-all invoke(method: string, ...) overload — that " +
			"accepts every mistyped command, which is what the per-command overloads exist to catch")
	}

	for _, real := range []string{"declare function getConfig()", "declare function createWindow(", "declare const goleo"} {
		if !strings.Contains(dts, real) {
			t.Errorf("init.d.ts does not declare %q, which the engine defines", real)
		}
	}
	// Declaring console would be a "cannot redeclare block-scoped variable" error in any
	// project including the DOM or ES libs, so its absence is deliberate and load-bearing.
	if strings.Contains(dts, "declare const console") || strings.Contains(dts, "declare var console") {
		t.Error("init.d.ts declares console — TypeScript's own lib already does, and redeclaring it " +
			"breaks any project that includes lib.dom or lib.es")
	}
	// The same fabrication guard as the comment block: types are documentation an editor
	// enforces, so a phantom here is worse than a phantom in prose.
	for _, claim := range []string{"declare const bridge", "declare function invoke", "goleo:getOS"} { // note: `goleo` is real; a bare `bridge` global never was
		if strings.Contains(dts, claim) {
			t.Errorf("init.d.ts declares %q, which the engine does not provide", claim)
		}
	}
	// createWindow returns false in browser mode — the gotcha the original docs missed.
	if !strings.Contains(dts, "): boolean") {
		t.Error("createWindow's declared return type is not boolean; browser mode returns false")
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

// scriptedApp wires a real App+Bridge to a scripted runtime, so JS -> Go goes through the
// same HandleRequestContext path production uses — Policy check included.
func scriptedApp(t *testing.T, script string) *App {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backend", "init.js"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	app := New(Config{DevMode: true, Title: "scripted", WindowMode: WindowModeBrowser})
	app.jsr = NewJSRuntime(app.config, app)
	if err := app.jsr.Run(); err != nil {
		t.Fatalf("init script failed: %v", err)
	}
	t.Cleanup(app.jsr.Stop)
	return app
}

func TestJSCanInvokeBridgeCommands(t *testing.T) {
	app := scriptedApp(t, `
	  function addViaGo(a, b) { return goleo.invoke("test:add", { a: a, b: b }); }
	  function callMissing() { return goleo.invoke("test:nope"); }
	`)
	app.Bridge().Handle("test:add", func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct{ A, B float64 }
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, err
		}
		return in.A + in.B, nil
	})

	ctx := context.Background()
	if v, err := app.JS().Call(ctx, "addViaGo", 2, 3); err != nil || v != int64(5) {
		t.Errorf("goleo.invoke round trip = %v (%T), %v; want 5", v, v, err)
	}
	// An unknown method must surface in JS as a throw the script can catch, not a silent nil.
	if _, err := app.JS().Call(ctx, "callMissing"); err == nil || !strings.Contains(err.Error(), "method not found") {
		t.Errorf("invoking an unregistered method should throw into JS, got: %v", err)
	}
}

// The reason J4 says route through HandleRequestContext rather than the handler map: the
// capability check then applies to scripts for free and cannot drift out of sync.
func TestJSInvokeIsGatedByPolicy(t *testing.T) {
	app := scriptedApp(t, `function secret() { return goleo.invoke("test:secret"); }`)
	app.Bridge().Handle("test:secret", func(ctx context.Context, raw json.RawMessage) (any, error) {
		return "leaked", nil
	})
	// Deny-by-default: a policy that does not list the method must refuse it.
	app.SetPolicy(&Policy{Allow: []string{"goleo:getOS"}})

	_, err := app.JS().Call(context.Background(), "secret")
	if err == nil {
		t.Fatal("Policy did not gate a JS-initiated invoke — the ACL is bypassed from scripts")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("expected the Policy denial to reach the script, got: %v", err)
	}
}

// The one hazard the single-goroutine design introduces: JS -> Go -> JS would have the
// owning goroutine waiting on itself. Without the inline marker this test hangs until the
// context deadline; with it, the nested call runs inline and returns.
func TestJSGoJSReentrancyDoesNotDeadlock(t *testing.T) {
	app := scriptedApp(t, `
	  function outer() { return goleo.invoke("test:reenter"); }
	  function inner() { return "from JS again"; }
	`)
	app.Bridge().Handle("test:reenter", func(ctx context.Context, raw json.RawMessage) (any, error) {
		// The handler calls BACK into JS on the goroutine already running this call.
		return app.JS().Call(ctx, "inner")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	v, err := app.JS().Call(ctx, "outer")
	if err != nil {
		t.Fatalf("JS -> Go -> JS deadlocked or failed: %v", err)
	}
	if v != "from JS again" {
		t.Errorf("re-entrant call returned %v", v)
	}
}

// goleo.emit pushes to the FRONTEND, which is the direction App.Emit goes.
//
// Worth stating because the naming invites the wrong assumption, and this test originally
// encoded it: Emit and On are NOT a pair. App.Emit fans out to Bridge subscribers (the
// WebSocket push path, i.e. backend -> frontend), while App.On registers Go handlers that
// DispatchEvent fires for events arriving FROM the frontend. A script emitting to itself
// via App.On would never be delivered, and the test asserting so was wrong rather than the
// code.
func TestJSCanEmitEventsToTheFrontend(t *testing.T) {
	app := scriptedApp(t, `function ping() { goleo.emit("script:ready", { ok: true }); }`)
	sub := app.Bridge().Subscribe()
	defer app.Bridge().Unsubscribe(sub)

	if _, err := app.JS().Call(context.Background(), "ping"); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-sub:
		if msg.Event != "script:ready" {
			t.Errorf("event name = %q", msg.Event)
		}
		if !strings.Contains(string(msg.Data), "true") {
			t.Errorf("payload did not survive the boundary: %s", msg.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("goleo.emit did not reach a frontend subscriber")
	}
}

// vanillaGlobalsJSON is the global set of a goja runtime with nothing installed, so the
// probe above can subtract it and see exactly what goleo contributes.
func vanillaGlobalsJSON(t *testing.T) string {
	t.Helper()
	vm := goja.New()
	v, err := vm.RunString(`JSON.stringify(Object.getOwnPropertyNames(this))`)
	if err != nil {
		t.Fatal(err)
	}
	return v.String()
}
