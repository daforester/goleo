package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// FSScope selects how strictly the filesystem plugin confines paths.
type FSScope int

const (
	// FSScopeStandard confines the fs plugin to the app's own data directory,
	// any Policy.FSRoots, and paths the user picked in a native dialog this
	// session. This is the zero value, so it is the default.
	FSScopeStandard FSScope = iota
	// FSScopeUnrestricted restores the historical behaviour: any absolute path
	// the OS will allow. Only for development tools and file managers that
	// genuinely need the whole disk. Also settable with GOLEO_FS_UNRESTRICTED=1.
	FSScopeUnrestricted
)

// fsScopeState is the confinement state for one Bridge's fs plugin.
//
// Before this existed, runtime/fs.validatePath rejected only *relative*
// traversal ("../x"), so every absolute path was allowed — and RegisterFS is part
// of the default desktop bundle enabled by both scaffolds. Any XSS, compromised
// npm/CDN dependency, or third-party script in the webview could therefore read
// ~/.ssh/id_rsa or hand os.RemoveAll a home directory. Policy.FSRoots looked like
// the mitigation but had no call sites at all.
type fsScopeState struct {
	mu     sync.RWMutex
	mode   FSScope
	roots  []string        // Policy.FSRoots + app data dir
	grants map[string]bool // paths the user chose in a dialog this session
	warned map[string]bool // out-of-scope reads already warned about
}

func newFSScopeState() *fsScopeState {
	return &fsScopeState{
		grants: map[string]bool{},
		warned: map[string]bool{},
	}
}

// SetFSScope selects the confinement mode for the fs plugin. App.New calls this
// from Config.FSScope; call it directly if you construct a Bridge yourself.
func (b *Bridge) SetFSScope(mode FSScope) {
	s := b.fsScope()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
}

// AddFSRoot adds a directory the fs plugin may reach. App.New adds the app's data
// directory; Policy.FSRoots are added when a policy is installed.
func (b *Bridge) AddFSRoot(dir string) {
	if dir == "" {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	s := b.fsScope()
	s.mu.Lock()
	defer s.mu.Unlock()
	// Store BOTH the literal and the symlink-resolved form. checkFSPath resolves
	// the requested path before comparing, so a root left unresolved would never
	// match: on macOS os.MkdirTemp and os.TempDir hand back /var/folders/... while
	// /var is a symlink to /private/var, so a resolved /private/var/... path was
	// compared against a /var/... root and every in-scope write was refused. Real
	// macOS hardware caught this; Windows and Linux did not, because their temp
	// and config dirs are not symlinks.
	for _, cand := range fsRootForms(abs) {
		if !containsPath(s.roots, cand) {
			s.roots = append(s.roots, cand)
		}
	}
}

// fsRootForms returns the distinct spellings of a root that a resolved request
// path might legitimately match: the path itself and its symlink-resolved form.
func fsRootForms(abs string) []string {
	forms := []string{abs}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && resolved != abs {
		forms = append(forms, resolved)
	}
	return forms
}

func containsPath(list []string, p string) bool {
	for _, e := range list {
		if e == p {
			return true
		}
	}
	return false
}

// GrantFSPath records a path the user explicitly chose in a native dialog, so the
// app may then read or write it even though it sits outside the configured roots.
// This is what keeps the ordinary "user picks a file, app opens it" flow working
// without any configuration — the same model Tauri uses.
func (b *Bridge) GrantFSPath(path string) {
	if path == "" {
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	s := b.fsScope()
	s.mu.Lock()
	defer s.mu.Unlock()
	// Both spellings, for the same reason as AddFSRoot: the request is compared
	// after symlink resolution.
	for _, form := range fsRootForms(abs) {
		s.grants[normalizeFSPath(form)] = true
	}
}

// fsDenied is refused in every mode, including FSScopeUnrestricted. These are
// paths where a write or delete damages the machine rather than the app's data,
// and no legitimate goleo app needs the plugin to reach them.
func fsDenied(abs string) bool {
	n := normalizeFSPath(abs)
	var roots []string
	if runtime.GOOS == "windows" {
		for _, env := range []string{"SystemRoot", "ProgramFiles", "ProgramFiles(x86)", "ProgramData"} {
			if v := os.Getenv(env); v != "" {
				roots = append(roots, normalizeFSPath(v))
			}
		}
	} else {
		roots = []string{"/system", "/usr", "/etc", "/bin", "/sbin", "/boot", "/lib", "/private/etc", "/library/startupitems"}
	}
	for _, r := range roots {
		if r != "" && (n == r || strings.HasPrefix(n, r+string(filepath.Separator))) {
			return true
		}
	}
	return false
}

// normalizeFSPath cleans a path and, on case-insensitive filesystems, lowercases
// it so prefix comparisons can't be bypassed by case.
func normalizeFSPath(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return p
}

// fsOp describes the kind of access being checked, because reads and writes are
// treated differently during the deprecation window.
type fsOp int

const (
	fsRead fsOp = iota
	fsWrite
)

// checkFSPath is the gate every fs handler goes through. It resolves path to an
// absolute, symlink-free location and decides whether the operation may proceed.
//
// Writes and deletes are confined immediately: an unrecoverable os.RemoveAll does
// not get a compatibility window. Reads outside the scope are still permitted for
// now but log a one-time warning naming the path, so existing apps keep working
// while the change is visible; they become errors in a later release.
func (b *Bridge) checkFSPath(path string, op fsOp) (string, error) {
	if path == "" {
		return "", fmt.Errorf("fs: path must not be empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("fs: resolving %q: %w", path, err)
	}
	// Resolve symlinks so a link inside an allowed root cannot point out of it.
	// A path that does not exist yet (a new file) is expected, so fall back to
	// resolving its parent.
	resolved := abs
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		resolved = r
	} else if parent, perr := filepath.EvalSymlinks(filepath.Dir(abs)); perr == nil {
		resolved = filepath.Join(parent, filepath.Base(abs))
	}

	if op == fsWrite && fsDenied(resolved) {
		return "", fmt.Errorf("fs: refusing to modify %q — system location", path)
	}

	s := b.fsScope()
	s.mu.RLock()
	mode, roots := s.mode, append([]string(nil), s.roots...)
	granted := s.grants[normalizeFSPath(resolved)] || s.grants[normalizeFSPath(abs)]
	s.mu.RUnlock()

	if mode == FSScopeUnrestricted || fsUnrestrictedEnv() {
		return resolved, nil
	}
	// Only the RESOLVED request may be compared against the roots. Matching the
	// unresolved path as well would reopen the symlink escape this check exists to
	// close: a link at <root>/link pointing outside still has <root>/ as a literal
	// prefix, so an unresolved comparison would admit it. Cross-spelling matching
	// belongs on the root side (fsRootForms), never on the request side.
	if granted || fsWithinRoots(resolved, roots) {
		return resolved, nil
	}

	if op == fsWrite {
		return "", fmt.Errorf("fs: %q is outside the allowed roots. Add it with Policy.FSRoots, "+
			"let the user pick it via a native dialog, or set Config.FSScope = FSScopeUnrestricted", path)
	}

	// Read outside scope: allow, but say so once per path.
	s.mu.Lock()
	first := !s.warned[normalizeFSPath(resolved)]
	if first {
		s.warned[normalizeFSPath(resolved)] = true
	}
	s.mu.Unlock()
	if first {
		fmt.Printf("  goleo: DEPRECATED — read outside the allowed filesystem roots: %s\n"+
			"    This will become an error. Add the directory to Policy.FSRoots, obtain the path\n"+
			"    from a native dialog, or set Config.FSScope = FSScopeUnrestricted.\n", resolved)
	}
	return resolved, nil
}

func fsWithinRoots(abs string, roots []string) bool {
	n := normalizeFSPath(abs)
	for _, r := range roots {
		rn := normalizeFSPath(r)
		if n == rn || strings.HasPrefix(n, rn+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func fsUnrestrictedEnv() bool {
	v := os.Getenv("GOLEO_FS_UNRESTRICTED")
	return v == "1" || strings.EqualFold(v, "true")
}
