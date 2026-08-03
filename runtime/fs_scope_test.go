package runtime

import (
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

func TestEmptyPathIsRejected(t *testing.T) {
	b := NewBridge()
	if _, err := b.checkFSPath("", fsRead); err == nil {
		t.Error("an empty path must be rejected")
	}
}
