package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// newScopedBridge returns a Bridge confined to root, mimicking what App.New does
// with the app data directory.
func newScopedBridge(t *testing.T, root string) *Bridge {
	t.Helper()
	b := NewBridge()
	b.AddFSRoot(root)
	return b
}

// The bug this phase closes: validatePath rejected only relative traversal, so
// every absolute path was allowed — and RegisterFS ships in the default desktop
// bundle that both scaffolds enable. A write or delete outside the scope must now
// be refused outright; there is no compatibility window for os.RemoveAll.
func TestWritesOutsideScopeAreRefused(t *testing.T) {
	root := t.TempDir()
	b := newScopedBridge(t, root)
	outside := filepath.Join(t.TempDir(), "victim.txt")

	if _, err := b.checkFSPath(outside, fsWrite); err == nil {
		t.Fatal("a write outside the allowed roots must be refused")
	} else if !strings.Contains(err.Error(), "outside the allowed roots") {
		t.Errorf("error should explain the refusal, got %v", err)
	}

	// Inside the root is fine, including a file that does not exist yet.
	if _, err := b.checkFSPath(filepath.Join(root, "new", "file.txt"), fsWrite); err != nil {
		t.Errorf("write inside the root should be allowed, got %v", err)
	}
}

// Reads get a deprecation window so existing apps keep working, but they must
// still be *reported*.
func TestReadsOutsideScopeWarnButSucceed(t *testing.T) {
	root := t.TempDir()
	b := newScopedBridge(t, root)
	outside := filepath.Join(t.TempDir(), "readable.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.checkFSPath(outside, fsRead); err != nil {
		t.Errorf("out-of-scope reads should still succeed during the deprecation window, got %v", err)
	}
}

// A path the user chose in a native dialog must be usable even though it is
// outside the configured roots — otherwise the ordinary "pick a file, then read
// it" flow would break for every app with no configuration.
func TestDialogGrantsMakePathsUsable(t *testing.T) {
	root := t.TempDir()
	b := newScopedBridge(t, root)
	picked := filepath.Join(t.TempDir(), "chosen.txt")

	if _, err := b.checkFSPath(picked, fsWrite); err == nil {
		t.Fatal("precondition: the path should start out of scope")
	}
	b.GrantFSPath(picked)
	if _, err := b.checkFSPath(picked, fsWrite); err != nil {
		t.Errorf("a user-granted path must be writable, got %v", err)
	}
}

// SelectFolder grants a whole directory, so files inside it work too.
func TestAddFSRootCoversChildren(t *testing.T) {
	root := t.TempDir()
	b := NewBridge()
	b.AddFSRoot(root)
	for _, p := range []string{
		filepath.Join(root, "a.txt"),
		filepath.Join(root, "nested", "deep", "b.txt"),
		root,
	} {
		if _, err := b.checkFSPath(p, fsWrite); err != nil {
			t.Errorf("%q should be in scope, got %v", p, err)
		}
	}
}

// Policy.FSRoots was inert — this is what makes it real.
func TestPolicyFSRootsAreEnforced(t *testing.T) {
	allowed := t.TempDir()
	b := NewBridge()
	target := filepath.Join(allowed, "data.json")

	if _, err := b.checkFSPath(target, fsWrite); err == nil {
		t.Fatal("precondition: path should be out of scope before the policy is set")
	}
	b.SetPolicy(&Policy{Allow: []string{"goleo:fs*"}, FSRoots: []string{allowed}})
	if _, err := b.checkFSPath(target, fsWrite); err != nil {
		t.Errorf("Policy.FSRoots must widen the scope, got %v", err)
	}
}

// Traversal must not escape a root, and a symlink pointing out of a root must not
// launder a path into it.
func TestScopeResistsTraversalAndSymlinks(t *testing.T) {
	root := t.TempDir()
	b := newScopedBridge(t, root)

	if _, err := b.checkFSPath(filepath.Join(root, "..", "escape.txt"), fsWrite); err == nil {
		t.Error("traversal out of the root must be refused")
	}

	// Symlink inside the root pointing outside it.
	outsideDir := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outsideDir, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err) // Windows without privilege
	}
	if _, err := b.checkFSPath(filepath.Join(link, "x.txt"), fsWrite); err == nil {
		t.Error("a symlink escaping the root must be refused")
	}
}

// Regression, found by the macos-14 CI runner: checkFSPath resolves the requested
// path through EvalSymlinks, so a root that was NOT resolved could never match it.
// On macOS os.MkdirTemp/os.TempDir return /var/folders/... while /var is a symlink
// to /private/var, so a resolved /private/var/... request was compared against a
// /var/... root and EVERY in-scope write was refused — the plugin was unusable
// there while still looking correct on Windows and Linux, whose temp and config
// directories are not symlinks.
func TestRootReachedThroughASymlinkStillMatches(t *testing.T) {
	real := t.TempDir()
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "aliased")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	// The app registers the *aliased* spelling, exactly as macOS hands it over.
	b := NewBridge()
	b.AddFSRoot(link)

	// A write through either spelling must be allowed — both name the same place.
	for _, p := range []string{
		filepath.Join(link, "notes.txt"),
		filepath.Join(real, "notes.txt"),
	} {
		if _, err := b.checkFSPath(p, fsWrite); err != nil {
			t.Errorf("%q is inside the registered root and must be writable, got %v", p, err)
		}
	}

	// And the reverse: registering the real path must accept the aliased request.
	b2 := NewBridge()
	b2.AddFSRoot(real)
	if _, err := b2.checkFSPath(filepath.Join(link, "x.txt"), fsWrite); err != nil {
		t.Errorf("aliased request into a real root must be allowed, got %v", err)
	}

	// Confinement must still hold — this fix widens spellings, not scope.
	if _, err := b.checkFSPath(filepath.Join(t.TempDir(), "outside.txt"), fsWrite); err == nil {
		t.Error("a genuinely out-of-scope path must still be refused")
	}
}

// Session grants have the same asymmetry: a granted path is compared post-resolution.
func TestGrantThroughASymlinkStillMatches(t *testing.T) {
	real := t.TempDir()
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "aliased")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	target := filepath.Join(link, "picked.txt")

	b := NewBridge()
	b.AddFSRoot(t.TempDir()) // some unrelated root
	if _, err := b.checkFSPath(target, fsWrite); err == nil {
		t.Fatal("precondition: should start out of scope")
	}
	b.GrantFSPath(target) // as a dialog would
	if _, err := b.checkFSPath(target, fsWrite); err != nil {
		t.Errorf("a granted aliased path must be writable, got %v", err)
	}
}

// The deny-list holds even in unrestricted mode: these are places where a write
// damages the machine, not the app's data.
func TestDenyListAppliesEvenWhenUnrestricted(t *testing.T) {
	b := NewBridge()
	b.SetFSScope(FSScopeUnrestricted)

	var target string
	if runtime.GOOS == "windows" {
		target = filepath.Join(os.Getenv("SystemRoot"), "System32", "drivers", "etc", "hosts")
	} else {
		target = "/etc/hosts"
	}
	if target == "" {
		t.Skip("no system path to test")
	}
	if _, err := b.checkFSPath(target, fsWrite); err == nil {
		t.Errorf("%q must be refused for writes even when unrestricted", target)
	}
	// Reads of system paths are not blocked by the deny-list.
	if _, err := b.checkFSPath(target, fsRead); err != nil {
		t.Errorf("reads should not be deny-listed, got %v", err)
	}
}

func TestUnrestrictedModeAllowsAnyPath(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "anything.txt")

	strict := NewBridge()
	strict.AddFSRoot(t.TempDir())
	if _, err := strict.checkFSPath(outside, fsWrite); err == nil {
		t.Fatal("precondition: strict mode should refuse this")
	}

	loose := NewBridge()
	loose.AddFSRoot(t.TempDir())
	loose.SetFSScope(FSScopeUnrestricted)
	if _, err := loose.checkFSPath(outside, fsWrite); err != nil {
		t.Errorf("FSScopeUnrestricted should allow any non-denied path, got %v", err)
	}
}

func TestUnrestrictedEnvEscapeHatch(t *testing.T) {
	b := NewBridge()
	b.AddFSRoot(t.TempDir())
	outside := filepath.Join(t.TempDir(), "x.txt")
	if _, err := b.checkFSPath(outside, fsWrite); err == nil {
		t.Fatal("precondition: should be refused without the env var")
	}
	t.Setenv("GOLEO_FS_UNRESTRICTED", "1")
	if _, err := b.checkFSPath(outside, fsWrite); err != nil {
		t.Errorf("GOLEO_FS_UNRESTRICTED=1 should lift confinement, got %v", err)
	}
}

// The default (zero value) must be the confined mode — safe by default is the
// whole point, and a Bridge built without SetFSScope must not be permissive.
func TestDefaultModeIsConfined(t *testing.T) {
	if FSScopeStandard != 0 {
		t.Error("FSScopeStandard must be the zero value so the default is safe")
	}
	b := NewBridge()
	b.AddFSRoot(t.TempDir())
	if _, err := b.checkFSPath(filepath.Join(t.TempDir(), "x"), fsWrite); err == nil {
		t.Error("a Bridge with no explicit scope call must still confine writes")
	}
}

// App.New must put the app's own data directory in scope, or an app cannot write
// its own files without configuration.
func TestAppDataDirIsInScopeByDefault(t *testing.T) {
	app := New(Config{Title: "ScopeTestApp", AppID: "scope-test-app"})
	base, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("no user config dir: %v", err)
	}
	target := filepath.Join(base, "scope-test-app", "state.json")
	if _, err := app.Bridge().checkFSPath(target, fsWrite); err != nil {
		t.Errorf("the app data dir must be writable by default, got %v", err)
	}
}

// End-to-end through the real bridge handlers, which is how a hostile script in
// the webview would reach this. Before the fix, `goleo:fsDelete` with an absolute
// path reached os.RemoveAll and `goleo:fsReadTextFile` read anything the user
// could — from any XSS or compromised npm dependency.
func TestFSHandlersRefuseOutOfScopeAccess(t *testing.T) {
	appRoot := t.TempDir()
	app := New(Config{Title: "FSHandlerTest", AppID: "fs-handler-test"})
	b := app.Bridge()
	b.AddFSRoot(appRoot)
	RegisterFS(b)

	// A file the app has no business touching.
	victimDir := t.TempDir()
	victim := filepath.Join(victimDir, "id_rsa")
	if err := os.WriteFile(victim, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatal(err)
	}

	call := func(method, argsJSON string) InvokeResponse {
		return b.HandleRequest(InvokeRequest{
			ID:     "1",
			Method: method,
			Args:   []byte(argsJSON),
		})
	}
	quoted := strings.ReplaceAll(victim, `\`, `\\`)

	// The delete must be refused, and the file must survive.
	if resp := call("goleo:fsDelete", `{"path":"`+quoted+`"}`); resp.Error == "" {
		t.Error("goleo:fsDelete outside the scope must return an error")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("the out-of-scope file was deleted: %v", err)
	}

	// Writing over it must be refused too.
	if resp := call("goleo:fsWriteTextFile", `{"path":"`+quoted+`","content":"pwned"}`); resp.Error == "" {
		t.Error("goleo:fsWriteTextFile outside the scope must return an error")
	}
	if data, _ := os.ReadFile(victim); string(data) != "PRIVATE KEY" {
		t.Error("the out-of-scope file was overwritten")
	}

	// Inside the app's own root everything still works.
	inScope := strings.ReplaceAll(filepath.Join(appRoot, "notes.txt"), `\`, `\\`)
	if resp := call("goleo:fsWriteTextFile", `{"path":"`+inScope+`","content":"hello"}`); resp.Error != "" {
		t.Errorf("in-scope write should succeed, got %q", resp.Error)
	}
	if resp := call("goleo:fsReadTextFile", `{"path":"`+inScope+`"}`); resp.Error != "" {
		t.Errorf("in-scope read should succeed, got %q", resp.Error)
	}
}

// The app data directory must resolve even when it becomes resolvable only AFTER
// App.New. On mobile os.UserConfigDir needs $HOME, which the gomobile host process
// does not have until the native shell calls SetHomeDir (MainActivity.java /
// AppDelegate.swift). That currently runs before app.New, but resolving eagerly
// would silently leave the plugin with no root if anyone reordered those calls —
// a security control should not depend on init order.
func TestAppDataRootResolvesLazily(t *testing.T) {
	// Point os.UserConfigDir somewhere that does not exist yet, mimicking a host
	// with no usable $HOME at construction time.
	base := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", filepath.Join(base, "missing"))
	case "darwin":
		t.Setenv("HOME", filepath.Join(base, "missing"))
	default:
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "missing"))
	}

	app := New(Config{Title: "LazyApp", AppID: "lazy-app"})

	// NOW make the config dir available — the "SetHomeDir arrives late" case.
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", base)
	case "darwin":
		t.Setenv("HOME", base)
	default:
		t.Setenv("XDG_CONFIG_HOME", base)
	}

	cfgBase, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("cannot direct os.UserConfigDir on this platform: %v", err)
	}
	target := filepath.Join(cfgBase, "lazy-app", "state.json")
	if _, err := app.Bridge().checkFSPath(target, fsWrite); err != nil {
		t.Errorf("app data dir must be in scope once resolvable, got %v", err)
	}
	// Still confined to it.
	if _, err := app.Bridge().checkFSPath(filepath.Join(base, "elsewhere.txt"), fsWrite); err == nil {
		t.Error("a sibling of the app data dir must still be out of scope")
	}
}

// goleo:fsAppDataDir vends a path as "where your data goes", so refusing writes to
// it afterwards is incoherent. It broke the scaffolded demo, whose FileSystem page
// does appDataDir("goleo-demo") while the scope root is named after the app's
// AppID/Title — different directory, so every write was refused. The handler now
// brings what it returns into scope.
func TestAppDataDirHandlerGrantsWhatItReturns(t *testing.T) {
	app := New(Config{Title: "DemoHost", AppID: "demo-host"})
	b := app.Bridge()
	RegisterFS(b)

	resp := b.HandleRequest(InvokeRequest{
		ID: "1", Method: "goleo:fsAppDataDir",
		Args: []byte(`{"appName":"goleo-demo"}`),
	})
	if resp.Error != "" {
		t.Fatalf("fsAppDataDir: %s", resp.Error)
	}
	dir, ok := resp.Result.(string)
	if !ok || dir == "" {
		t.Fatalf("expected a directory, got %#v", resp.Result)
	}

	// The demo then writes a file there — this must work.
	target := filepath.Join(dir, "demo.txt")
	if _, err := b.checkFSPath(target, fsWrite); err != nil {
		t.Errorf("a path returned by fsAppDataDir must be writable, got %v", err)
	}
}

// ...but appName becomes a path element, so it must not be usable to climb out
// and turn this into an arbitrary-directory grant.
func TestAppDataDirHandlerRejectsEscapingNames(t *testing.T) {
	app := New(Config{Title: "EscapeHost", AppID: "escape-host"})
	b := app.Bridge()
	RegisterFS(b)

	for _, name := range []string{
		"../../etc", "..", ".", "a/b", `a\b`, "/abs", "../sneaky", "x/../../y",
	} {
		resp := b.HandleRequest(InvokeRequest{
			ID: "1", Method: "goleo:fsAppDataDir",
			Args: []byte(`{"appName":` + fmt.Sprintf("%q", name) + `}`),
		})
		if resp.Error == "" {
			t.Errorf("appName %q should be rejected, got result %#v", name, resp.Result)
		}
	}

	// A plain name is still fine.
	if resp := b.HandleRequest(InvokeRequest{
		ID: "1", Method: "goleo:fsAppDataDir", Args: []byte(`{"appName":"my-app"}`),
	}); resp.Error != "" {
		t.Errorf("a plain appName should be accepted, got %s", resp.Error)
	}
}

// fsHomeDir must stay informational — granting it would hand back the whole user
// profile and defeat the confinement entirely.
func TestHomeDirHandlerGrantsNothing(t *testing.T) {
	app := New(Config{Title: "HomeHost", AppID: "home-host"})
	b := app.Bridge()
	RegisterFS(b)

	resp := b.HandleRequest(InvokeRequest{ID: "1", Method: "goleo:fsHomeDir"})
	if resp.Error != "" {
		t.Skipf("no home dir here: %s", resp.Error)
	}
	home, _ := resp.Result.(string)
	if home == "" {
		t.Skip("no home dir reported")
	}
	if _, err := b.checkFSPath(filepath.Join(home, ".ssh", "id_rsa"), fsWrite); err == nil {
		t.Error("fsHomeDir must not bring the home directory into scope")
	}
}

// Replays the scaffolded demo's FileSystemDemo.vue sequence through the real
// bridge, because that page is the first thing anyone clicks and it is what caught
// the fsAppDataDir problem. Order matters: appDataDir() is what brings the
// directory into scope, and write() calls ensureDir() before writing.
func TestDemoFileSystemFlowWorksEndToEnd(t *testing.T) {
	app := New(Config{Title: "Demo", AppID: "demo-flow"})
	b := app.Bridge()
	RegisterFS(b)

	call := func(method, args string) InvokeResponse {
		return b.HandleRequest(InvokeRequest{ID: "1", Method: method, Args: []byte(args)})
	}

	// ensureDir(): appDataDir('goleo-demo')
	resp := call("goleo:fsAppDataDir", `{"appName":"goleo-demo"}`)
	if resp.Error != "" {
		t.Fatalf("appDataDir: %s", resp.Error)
	}
	dir := resp.Result.(string)
	file := strings.ReplaceAll(filepath.Join(dir, "goleo-demo.txt"), `\`, `\\`)
	t.Cleanup(func() { os.RemoveAll(dir) })

	// write()
	if r := call("goleo:fsWriteTextFile", `{"path":"`+file+`","content":"hello demo"}`); r.Error != "" {
		t.Fatalf("write: %s", r.Error)
	}
	// read()
	if r := call("goleo:fsReadTextFile", `{"path":"`+file+`"}`); r.Error != "" {
		t.Errorf("read: %s", r.Error)
	} else if r.Result != "hello demo" {
		t.Errorf("read returned %#v", r.Result)
	}
	// list()
	if r := call("goleo:fsListDir", `{"path":"`+strings.ReplaceAll(dir, `\`, `\\`)+`"}`); r.Error != "" {
		t.Errorf("listDir: %s", r.Error)
	}
	// remove()
	if r := call("goleo:fsDelete", `{"path":"`+file+`"}`); r.Error != "" {
		t.Errorf("delete: %s", r.Error)
	}
}

// The demo's read()/remove() do NOT call ensureDir first, so on a fresh page they
// pass "./goleo-demo.txt" — a RELATIVE path, which resolves against the backend's
// working directory rather than the app data dir. Confinement must refuse the
// delete rather than removing a file from the project directory, which is what the
// old unconfined code did.
func TestDemoRelativePathBeforeResolveIsRefused(t *testing.T) {
	app := New(Config{Title: "Demo", AppID: "demo-rel"})
	b := app.Bridge()
	RegisterFS(b)

	// Run from a directory that is definitely not in scope.
	t.Chdir(t.TempDir())

	r := b.HandleRequest(InvokeRequest{
		ID: "1", Method: "goleo:fsDelete", Args: []byte(`{"path":"./goleo-demo.txt"}`),
	})
	if r.Error == "" {
		t.Error("a relative path outside the scope must be refused for delete")
	}
}

// The refusal message is the only thing a developer sees when confinement bites,
// so its content is part of the contract. It must name the offending path and every
// way to allow it — a bare "permission denied" would send people guessing. (The
// bridge's fs.ts used to discard this entirely and substitute "requires the Go
// backend", which pointed at the wrong problem; it now rethrows the original.)
func TestRefusalMessageIsActionable(t *testing.T) {
	b := NewBridge()
	b.AddFSRoot(t.TempDir())
	offending := filepath.Join(t.TempDir(), "victim.txt")

	_, err := b.checkFSPath(offending, fsWrite)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	msg := err.Error()
	for _, want := range []string{
		filepath.Base(offending), // names what was refused
		"Policy.FSRoots",         // remedy 1
		"dialog",                 // remedy 2
		"FSScopeUnrestricted",    // remedy 3
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal message should mention %q; got: %s", want, msg)
		}
	}
}

func TestValidAppDataName(t *testing.T) {
	for _, ok := range []string{"goleo", "my-app", "My_App.v2", "goleo-demo"} {
		if !validAppDataName(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x", "a/b", `a\b`, "/x", "x/..", "..x/../y"} {
		if validAppDataName(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestEmptyPathIsRejected(t *testing.T) {
	b := NewBridge()
	if _, err := b.checkFSPath("", fsRead); err == nil {
		t.Error("an empty path must be rejected")
	}
}
