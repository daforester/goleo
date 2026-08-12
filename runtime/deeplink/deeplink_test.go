package deeplink

import (
	"strings"
	"testing"
)

func TestSchemeURL(t *testing.T) {
	args := []string{"--flag", "myapp://open/thing?x=1", "other"}
	if got := SchemeURL("myapp", args); got != "myapp://open/thing?x=1" {
		t.Errorf("SchemeURL = %q", got)
	}
	if got := SchemeURL("other", args); got != "" {
		t.Errorf("wrong scheme should not match: %q", got)
	}
	if got := SchemeURL("", args); got != "" {
		t.Errorf("empty scheme should not match: %q", got)
	}
	if got := SchemeURL("myapp", []string{"nothing"}); got != "" {
		t.Errorf("no url should return empty: %q", got)
	}
}

// TestSlug is deliberately not build-tagged even though slug's only caller is
// Linux-only: an untagged test references the helper on every platform, which is
// what stops a single-GOOS "unused code" sweep from reporting it as dead. It was
// deleted once on exactly that reasoning, and the Linux build broke.
func TestSlug(t *testing.T) {
	cases := map[string]string{"My App": "my-app", "Goleo!!": "goleo", "  x y ": "x-y"}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q)=%q want %q", in, got, want)
		}
	}
}

func TestDesktopEntry(t *testing.T) {
	e := desktopEntry("myapp", "My App", "/usr/bin/myapp")
	for _, want := range []string{"MimeType=x-scheme-handler/myapp;", "Exec=/usr/bin/myapp %u", "Name=My App"} {
		if !strings.Contains(e, want) {
			t.Errorf("desktop entry missing %q:\n%s", want, e)
		}
	}
}
