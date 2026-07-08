package runtime

import (
	"os"
	"path/filepath"
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
