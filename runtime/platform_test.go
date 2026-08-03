package runtime

import (
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

	// It should now pass the allow-list. We assert on the *scheme check* rather
	// than the spawn: exec.Command may legitimately fail in a headless test
	// environment, so only a scheme rejection counts as a failure here.
	if err := OpenURL("myapp://open/thing"); err != nil && strings.Contains(err.Error(), "not allowed") {
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
