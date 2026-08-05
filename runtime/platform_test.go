package runtime

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// goleo:openURL is a default builtin reachable by any script in the webview, and
// the OS handlers act on far more than web links: file:// and UNC paths expose
// the filesystem, and a path to an executable becomes arbitrary execution. These
// must be refused before reaching exec.Command.
func TestOpenURLRejectsUnsafeSchemes(t *testing.T) {
	unsafe := []string{
		"file:///etc/passwd",
		`file:///C:/Windows/System32/calc.exe`,
		`\\attacker\share\payload.exe`,
		"//attacker/share",
		`C:\Windows\System32\calc.exe`,
		"/usr/bin/id",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"vbscript:msgbox(1)",
		"ms-msdt:/id",
		"search-ms:query=x",
		"./relative/path",
	}
	for _, u := range unsafe {
		if err := OpenURL(u); err == nil {
			t.Errorf("OpenURL(%q) = nil, want a rejection", u)
		}
	}
}

// The rejection must be self-explanatory — this is a behaviour change, and a
// developer whose link stopped opening needs to know why and what to do.
func TestOpenURLRejectionMentionsAllowedSchemes(t *testing.T) {
	err := OpenURL("javascript:alert(1)")
	if err == nil {
		t.Fatal("expected a rejection")
	}
	msg := err.Error()
	for _, want := range []string{"javascript", "http", "https", "Config.URLScheme"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should mention %q", msg, want)
		}
	}
}

// AllowURLScheme is how an app opts its own deep-link scheme in; App.Run calls it
// for Config.URLScheme. Without it, an app could not open its own links.
func TestAllowURLSchemeOptsInAppScheme(t *testing.T) {
	// Sanity: unknown scheme rejected first.
	if err := OpenURL("myapp://open/thing"); err == nil {
		t.Fatal("precondition: an unregistered scheme should be rejected")
	}

	AllowURLScheme("myapp")
	t.Cleanup(func() {
		openURLMu.Lock()
		delete(openURLExtraSchemes, "myapp")
		openURLMu.Unlock()
	})

	// It should now pass the allow-list. Assert that through checkOutboundURL — the
	// guard OpenURL delegates to — and NOT by calling OpenURL.
	//
	// Calling OpenURL here used to actually launch the OS handler, because the scheme
	// had just been allowed. Nothing is registered for a test scheme, so Windows
	// popped its "How do you want to open this file?" chooser on every
	// `go test ./runtime/`, which is how this was noticed. The spawn adds nothing:
	// openURLCommand covers the argv, and this covers the allow-list.
	if err := checkOutboundURL("openURL", "myapp://open/thing"); err != nil {
		t.Errorf("registered scheme should pass the allow-list, got %v", err)
	}

	// A trailing "://" in the registration is tolerated, and matching is
	// case-insensitive (net/url lowercases the scheme).
	AllowURLScheme("Other://")
	t.Cleanup(func() {
		openURLMu.Lock()
		delete(openURLExtraSchemes, "other")
		openURLMu.Unlock()
	})
	if !allowedURLScheme("other") {
		t.Error(`AllowURLScheme("Other://") should register "other"`)
	}
}

func TestAllowedURLSchemeDefaults(t *testing.T) {
	for _, s := range []string{"http", "https", "mailto", "tel", "HTTPS"} {
		if !allowedURLScheme(s) {
			t.Errorf("%q should be allowed by default", s)
		}
	}
	for _, s := range []string{"file", "javascript", "data", "smb", "c", ""} {
		if allowedURLScheme(s) {
			t.Errorf("%q should not be allowed by default", s)
		}
	}
}

// goleo:share reaches the same OS default handlers as goleo:openURL
// (rundll32 url.dll / open / xdg-open), so it needs the same guard. It did not
// have one: openURL was hardened and share, which was written from the same
// mechanism, kept passing the frontend's URL straight through. That made
// {"url":"file:///C:/Windows/System32/calc.exe"} arbitrary execution from any
// script in the webview, and the demo scaffold enables RegisterShare.
//
// Driven through the bridge handler rather than checkOutboundURL directly, so the
// wiring is covered and not just the validator.
func TestShareRefusesNonWebURLs(t *testing.T) {
	b := NewBridge()
	RegisterShare(b)

	// Register a fake provider so an ALLOWED url never reaches the real OS
	// handler. share.Share prefers a provider over platformShare, and without one
	// this test genuinely launches things: verifying it by removing the guard
	// spawned two Calculator windows and a pair of rundll32 processes, and even
	// passing it would open three browser tabs on every `go test ./...`. A test for
	// a guard must not need the thing it guards to fire.
	var shared []string
	SetShareProvider(fakeSharer{&shared})
	t.Cleanup(func() { SetShareProvider(nil) })

	call := func(t *testing.T, url string) error {
		t.Helper()
		args, err := json.Marshal(map[string]string{"url": url, "text": "hi"})
		if err != nil {
			t.Fatal(err)
		}
		resp := b.HandleRequest(InvokeRequest{
			ID: "1", Method: "goleo:share", Args: args,
		})
		if resp.Error != "" {
			return errors.New(resp.Error)
		}
		return nil
	}

	// Every one of these is a path to arbitrary execution or a credential leak if
	// handed to the OS handler.
	for _, bad := range []string{
		"file:///C:/Windows/System32/calc.exe",
		"file:///etc/passwd",
		`\\attacker.example\share\payload.exe`,
		"//attacker.example/share",
		`C:\Windows\System32\calc.exe`,
		"/tmp/payload",
		"vbscript:msgbox(1)",
		"javascript:alert(1)",
		"ms-msdt:/id",
		"smb://attacker.example/share",
	} {
		if err := call(t, bad); err == nil {
			t.Errorf("share accepted %q — it reaches the OS default handler", bad)
		}
	}

	if len(shared) != 0 {
		t.Errorf("a refused url still reached the share backend: %v", shared)
	}

	// And it must still do its actual job for web links — reaching the backend with
	// the url intact, which is a stronger check than "returned no error".
	want := []string{"https://example.com/x", "http://example.com", "mailto:a@b.c"}
	for _, ok := range want {
		if err := call(t, ok); err != nil {
			t.Errorf("share refused the legitimate URL %q: %v", ok, err)
		}
	}
	if len(shared) != len(want) {
		t.Errorf("share backend received %v, want all of %v", shared, want)
	}
}

// fakeSharer stands in for the platform share backend so tests never invoke a real
// OS handler.
type fakeSharer struct{ got *[]string }

func (f fakeSharer) Share(d *ShareData) error {
	*f.got = append(*f.got, d.URL)
	return nil
}

// The two guards must stay one implementation. If a third caller appears, it has
// to go through the same function rather than growing its own copy.
func TestOpenURLAndShareShareOneGuard(t *testing.T) {
	const bad = "file:///etc/passwd"
	if err := checkOutboundURL("openURL", bad); err == nil {
		t.Fatal("checkOutboundURL accepted a file:// URL")
	}
	if err := OpenURL(bad); err == nil {
		t.Error("OpenURL accepted a file:// URL")
	}
	// A scheme registered by the app must be honoured by both. Unregister it after:
	// openURLExtraSchemes is process-global, and TestAllowURLSchemeOptsInAppScheme
	// asserts as a precondition that "myapp" is NOT registered. Leaving it behind
	// makes that test pass or fail on declaration order.
	AllowURLScheme("myapp")
	t.Cleanup(func() {
		openURLMu.Lock()
		delete(openURLExtraSchemes, "myapp")
		openURLMu.Unlock()
	})
	if err := checkOutboundURL("share", "myapp://open/thing"); err != nil {
		t.Errorf("a registered app scheme should be allowed: %v", err)
	}
}

// Covers what the removed spawn used to exercise implicitly, without launching
// anything. Same properties as share: the right handler per OS, no shell anywhere, and
// the URL as exactly one unmodified argv element.
func TestOpenURLCommandPerOSAndNoShell(t *testing.T) {
	const url = "https://example.com/x?a=1&b=2"

	for goos, want := range map[string][]string{
		"windows": {"rundll32", "url.dll,FileProtocolHandler", url},
		"darwin":  {"open", url},
		"linux":   {"xdg-open", url},
	} {
		name, args, err := openURLCommand(goos, url)
		if err != nil {
			t.Errorf("%s: unexpected error %v", goos, err)
			continue
		}
		got := append([]string{name}, args...)
		if len(got) != len(want) {
			t.Errorf("%s: argv = %q, want %q", goos, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: argv[%d] = %q, want %q", goos, i, got[i], want[i])
			}
		}
	}

	// An unknown platform must report it rather than guessing a handler.
	if _, _, err := openURLCommand("plan9", url); err == nil {
		t.Error("openURLCommand should refuse an unsupported platform")
	}

	// The `&` in the URL above is why this matters: no shell may be involved, or the
	// OS handler would re-parse it. (share had exactly this bug via `cmd /c start`.)
	for _, goos := range []string{"windows", "darwin", "linux"} {
		name, args, _ := openURLCommand(goos, url)
		for _, tok := range append([]string{name}, args...) {
			for _, shell := range []string{"cmd", "cmd.exe", "powershell", "pwsh", "sh", "bash", "/c", "-c"} {
				if strings.EqualFold(tok, shell) {
					t.Errorf("%s: argv contains shell token %q", goos, tok)
				}
			}
		}
		count := 0
		for _, a := range args {
			if a == url {
				count++
			}
		}
		if count != 1 {
			t.Errorf("%s: url appears %d times as its own argv element (args=%q)", goos, count, args)
		}
	}
}

// A standing guard against reintroducing the popup. Any test that reaches
// exec.Command in this package spawns something on the developer's machine — a
// browser tab, a Calculator window, or Windows' "how do you want to open this?"
// chooser. Both offenders so far were tests calling a function whose *last* step is
// the spawn, when the property under test was decided earlier.
//
// So: OpenURL must reject before spawning for everything the guard refuses, and the
// allowed path must be asserted through checkOutboundURL/openURLCommand instead. This
// test documents that rule where a future author will see it.
func TestNothingInThisPackageSpawnsAHandler(t *testing.T) {
	// Rejected URLs never reach exec.Command — safe to call for real.
	if err := OpenURL("file:///etc/passwd"); err == nil {
		t.Error("a refused URL must not reach the OS handler")
	}
	// The allowed path is covered by openURLCommand + checkOutboundURL, both pure.
	// If you are about to add `OpenURL(<something allowed>)` to a test: don't.
	if _, _, err := openURLCommand("linux", "https://example.com"); err != nil {
		t.Errorf("openURLCommand should cover the allowed path without spawning: %v", err)
	}
}
