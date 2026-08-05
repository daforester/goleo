package share

import (
	"errors"
	"strings"
	"testing"
)

// The package had no tests at all, which is why the Windows shell injection it was
// fixed for could only be reviewed, not verified.
//
// The dangerous shape is `cmd /c start <url>`: cmd re-parses its own command line and
// treats `&` as a command separator, and Go's syscall.EscapeArg quotes only
// arguments containing space, tab, newline or quote — not `&`. So a frontend-supplied
// `http://x&calc` executed calc. Nothing about a working share reveals this, so it
// needs a test that names the property rather than the symptom.
func TestShareCommandNeverInvokesAShell(t *testing.T) {
	// Payloads that a shell would act on and a plain argv element would not.
	hostile := []string{
		"http://example.com&calc",
		"http://example.com&&calc.exe",
		`http://example.com" & calc & "`,
		"http://example.com|calc",
		"http://example.com;id",
		"http://example.com$(id)",
		"http://example.com`id`",
		"http://example.com%0Acalc",
		"http://example.com\nid",
		"http://example.com >out.txt",
	}

	// Every desktop, from any host — the whole point of taking goos as a parameter.
	for _, goos := range []string{"windows", "darwin", "linux", "freebsd"} {
		for _, url := range hostile {
			name, args := osShareCommand(goos, url)

			// No shell interpreter may appear anywhere in the argv.
			for _, shell := range []string{"cmd", "cmd.exe", "powershell", "pwsh", "sh", "bash", "/c", "-c"} {
				if strings.EqualFold(name, shell) {
					t.Errorf("%s: command is the shell %q", goos, name)
				}
				for _, a := range args {
					if strings.EqualFold(a, shell) {
						t.Errorf("%s: argv contains shell token %q (argv=%q)", goos, a, args)
					}
				}
			}

			// The URL must survive as EXACTLY ONE argv element, unmodified. If it were
			// split or concatenated into another argument, the receiving program would
			// re-parse it.
			count := 0
			for _, a := range args {
				if a == url {
					count++
				}
			}
			if count != 1 {
				t.Errorf("%s: url %q appears %d times as its own argv element (argv=%q)",
					goos, url, count, args)
			}
		}
	}
}

// The per-OS handler must be the right one; a wrong command silently does nothing.
func TestShareCommandPerOS(t *testing.T) {
	const url = "https://example.com/x"
	cases := map[string]struct {
		name string
		args []string
	}{
		"windows": {"rundll32", []string{"url.dll,FileProtocolHandler", url}},
		"darwin":  {"open", []string{url}},
		"linux":   {"xdg-open", []string{url}},
	}
	for goos, want := range cases {
		name, args := osShareCommand(goos, url)
		if name != want.name {
			t.Errorf("%s: command = %q, want %q", goos, name, want.name)
		}
		if len(args) != len(want.args) {
			t.Fatalf("%s: argv = %q, want %q", goos, args, want.args)
		}
		for i := range args {
			if args[i] != want.args[i] {
				t.Errorf("%s: argv[%d] = %q, want %q", goos, i, args[i], want.args[i])
			}
		}
	}
}

// A registered provider must win over the platform handler. Mobile shells depend on
// it, and every test that would otherwise spawn a real handler depends on it too.
func TestProviderTakesPrecedenceOverThePlatformHandler(t *testing.T) {
	var got *ShareData
	SetProvider(fakeProvider{func(d *ShareData) error { got = d; return nil }})
	t.Cleanup(func() { SetProvider(nil) })

	in := &ShareData{Title: "t", Text: "x", URL: "https://example.com"}
	if err := Share(in); err != nil {
		t.Fatalf("Share: %v", err)
	}
	if got == nil {
		t.Fatal("the provider was not called — Share fell through to the platform handler")
	}
	if got.URL != in.URL || got.Text != in.Text || got.Title != in.Title {
		t.Errorf("provider got %+v, want %+v", got, in)
	}
}

// A text-only share has no desktop path. It must report ErrUnsupported so callers can
// errors.Is it and fall back to the Web Share API, rather than reading as a failure.
func TestTextOnlyShareIsUnsupportedNotAnError(t *testing.T) {
	SetProvider(nil)
	err := Share(&ShareData{Text: "just text"})
	if err == nil {
		t.Fatal("text-only share should not succeed on desktop")
	}
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("error should wrap errors.ErrUnsupported so the TS layer can fall back, got %v", err)
	}
}

type fakeProvider struct{ fn func(*ShareData) error }

func (f fakeProvider) Share(d *ShareData) error { return f.fn(d) }
