package autostart

import (
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	cases := map[string]string{"My App": "my-app", "Goleo!!": "goleo", "  x y ": "x-y"}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q)=%q want %q", in, got, want)
		}
	}
}

func TestDesktopEntry(t *testing.T) {
	e := desktopEntry("My App", "/usr/bin/myapp")
	for _, want := range []string{"[Desktop Entry]", "Name=My App", "Exec=/usr/bin/myapp", "X-GNOME-Autostart-enabled=true"} {
		if !strings.Contains(e, want) {
			t.Errorf("desktop entry missing %q:\n%s", want, e)
		}
	}
}

func TestLaunchAgentPlist(t *testing.T) {
	p := launchAgentPlist("com.goleo.myapp", "/Applications/My.app/Contents/MacOS/my")
	for _, want := range []string{"com.goleo.myapp", "/Applications/My.app/Contents/MacOS/my", "RunAtLoad", "ProgramArguments"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q:\n%s", want, p)
		}
	}
}
