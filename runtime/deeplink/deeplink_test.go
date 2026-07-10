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

func TestDesktopEntry(t *testing.T) {
	e := desktopEntry("myapp", "My App", "/usr/bin/myapp")
	for _, want := range []string{"MimeType=x-scheme-handler/myapp;", "Exec=/usr/bin/myapp %u", "Name=My App"} {
		if !strings.Contains(e, want) {
			t.Errorf("desktop entry missing %q:\n%s", want, e)
		}
	}
}
