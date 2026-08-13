package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// The whole of T2 turns on one distinction: a chrome property nobody set is not the same
// as one set to false. Every OS default here is the true-ish one (windows are resizable
// and decorated), so the moment "unset" collapses into "false" every goleo app loses its
// title bar. These tests exist to keep that collapse from happening quietly, on each of
// the four surfaces the value crosses: Config, createWindow's options object, an
// OpenWindow call, and the env var that carries it to a child-process window.

func TestMergeChromeOverridesFieldByField(t *testing.T) {
	base := WindowChrome{Resizable: Bool(false), Decorations: Bool(false)}
	over := WindowChrome{Decorations: Bool(true), AlwaysOnTop: Bool(true)}

	got := mergeChrome(base, over)

	if got.Resizable == nil || *got.Resizable {
		t.Error("Resizable was set only in base, so it should survive the merge as false")
	}
	if got.Decorations == nil || !*got.Decorations {
		t.Error("Decorations was set in both, so the override should win")
	}
	if got.AlwaysOnTop == nil || !*got.AlwaysOnTop {
		t.Error("AlwaysOnTop was set only in the override, so it should be applied")
	}
	if got.Fullscreen != nil {
		t.Error("Fullscreen was set in neither, so it must stay nil — a merge that " +
			"materialises false here would force every window out of fullscreen")
	}
}

// The bug this prevents is specific: reading these with getJSBool and a false default
// would strip the frame off every window created by a script that never mentioned
// decorations, which is every scaffolded app.
func TestChromeFromJSDistinguishesAbsentFromFalse(t *testing.T) {
	vm := goja.New()
	v, err := vm.RunString(`({ resizable: false, alwaysOnTop: true })`)
	if err != nil {
		t.Fatal(err)
	}
	got := chromeFromJS(v.ToObject(vm))

	if got.Resizable == nil {
		t.Fatal("resizable: false was dropped — an explicit false must be carried through")
	}
	if *got.Resizable {
		t.Error("resizable: false parsed as true")
	}
	if got.AlwaysOnTop == nil || !*got.AlwaysOnTop {
		t.Error("alwaysOnTop: true was not parsed")
	}
	if got.Decorations != nil || got.Fullscreen != nil {
		t.Errorf("properties the script never wrote must stay nil, got decorations=%v fullscreen=%v",
			got.Decorations, got.Fullscreen)
	}
}

// A child window inherits the app's chrome the same way it already inherits Title/Width/
// Height, and overrides it per field.
func TestResolveWindowOptionsInheritsConfigChrome(t *testing.T) {
	app := New(Config{
		Title:  "parent",
		Width:  800,
		Height: 600,
		Chrome: WindowChrome{Decorations: Bool(false), AlwaysOnTop: Bool(true)},
	})

	r := resolveWindowOptions(app, WindowOptions{
		Path:   "/settings",
		Chrome: WindowChrome{AlwaysOnTop: Bool(false)},
	})

	if r.Title != "parent" || r.Width != 800 || r.Height != 600 {
		t.Errorf("existing defaults regressed: %+v", r)
	}
	if r.Chrome.Decorations == nil || *r.Chrome.Decorations {
		t.Error("a frameless app should open frameless child windows (inherited from Config.Chrome)")
	}
	if r.Chrome.AlwaysOnTop == nil || *r.Chrome.AlwaysOnTop {
		t.Error("the window's own alwaysOnTop: false should override the app's true")
	}
}

// Child windows are separate processes, so chrome has to survive a trip through the
// environment. Three states per field, one variable.
func TestChromeEnvRoundTrip(t *testing.T) {
	in := WindowChrome{Resizable: Bool(false), Fullscreen: Bool(true)}

	enc, err := encodeChromeEnv(in)
	if err != nil {
		t.Fatal(err)
	}
	got := decodeChromeEnv(enc)

	if got.Resizable == nil || *got.Resizable {
		t.Error("resizable=false did not survive the env round-trip")
	}
	if got.Fullscreen == nil || !*got.Fullscreen {
		t.Error("fullscreen=true did not survive the env round-trip")
	}
	if got.Decorations != nil || got.AlwaysOnTop != nil {
		t.Error("unset fields must decode back to nil, not false")
	}

	// Nothing to say means no variable, rather than an empty one the child must parse.
	if enc, err := encodeChromeEnv(WindowChrome{}); err != nil || enc != "" {
		t.Errorf("zero chrome should encode to \"\", got %q (err %v)", enc, err)
	}
	// A malformed value is a goleo bug, not user input; the child still opens a window.
	if got := decodeChromeEnv("{not json"); !got.isZero() {
		t.Error("a malformed chrome env var should degrade to OS defaults, not a half-parsed window")
	}
}

// The fourth face of the createWindow contract, alongside the three in jsruntime_test.go
// (the VM's globals, the scaffold comment blocks, the generated .d.ts). Those hold the set
// of GLOBALS in sync; this holds the set of OPTIONS in sync, which is where the chrome
// fields actually live. A field the VM reads but nothing documents is invisible; a field
// documented but not read is the repo's recurring defect shape.
func TestChromeOptionsAreDocumentedEverywhereTheyAreRead(t *testing.T) {
	// The JSON tags are the JS property names by construction — same struct, same
	// createWindow opts, same WindowOptions.Chrome payload.
	fields := []string{"resizable", "alwaysOnTop", "fullscreen", "decorations"}

	for _, src := range []string{
		filepath.Join("..", "cli", "cmd", "generate.go"),                             // init.d.ts (backend script types)
		filepath.Join("..", "cli", "cmd", "templates.go"),                            // minimal scaffold's init.js
		filepath.Join("..", "cli", "cmd", "templates", "demo", "backend", "init.js"), // demo scaffold's init.js
		filepath.Join("..", "cli", "cmd", "schema.go"),                               // goleo:windowOpen's generated arg type
		filepath.Join("..", "bridge", "src", "window.ts"),                            // openWindow's hand-written type
	} {
		raw, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		body := string(raw)
		for _, f := range fields {
			if !strings.Contains(body, f) {
				t.Errorf("%s does not mention the createWindow option %q, which the JS engine reads "+
					"(chromeFromJS) — a window option nobody documents is one nobody can use", src, f)
			}
		}
	}
}
