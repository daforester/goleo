package notify

import "testing"

// goleo:notify is a DEFAULT builtin, so title and body are reachable by any script in
// the webview. notify-send uses GLib option parsing, so before the `--` terminator a
// title beginning with `--` was read as an option:
//
//   - "--help" / "--version": notify-send prints and exits 0, so the notification
//     silently never appears while Notify() reports success — the worst kind of
//     failure, because the app has no way to tell.
//   - "--icon=", "--hint=", "--expire-time=", "--urgency=": a frontend restyles,
//     retimes or suppresses notifications the app believes it controls.
//
// Same class as the zenity fix in runtime/dialogs, in the package whose darwin
// variant was hardened against AppleScript injection while this went unnoticed. No
// shell is involved, so flag confusion rather than RCE.
func TestNotifySendTerminatesOptionParsing(t *testing.T) {
	hostile := []string{
		"--help",
		"--version",
		"--icon=/tmp/evil.png",
		"--hint=string:image-path:/tmp/evil.png",
		"--expire-time=0",
		"--urgency=low",
		"--app-name=SomethingElse",
		"-u",
		"--",
		"-",
	}

	for _, v := range hostile {
		for _, where := range []string{"title", "body"} {
			var args []string
			if where == "title" {
				args = notifySendArgs(v, "body")
			} else {
				args = notifySendArgs("title", v)
			}

			// A `--` must appear before any frontend value, so nothing after it is
			// parsed as an option however it starts.
			term := -1
			for i, a := range args {
				if a == "--" {
					term = i
					break
				}
			}
			if term < 0 {
				t.Fatalf("%s=%q: argv has no `--` terminator: %q", where, v, args)
			}
			for i, a := range args {
				if a == v && i < term {
					t.Errorf("%s=%q appears at index %d, before the `--` at %d: %q",
						where, v, i, term, args)
				}
			}
		}
	}
}

// The values must still arrive intact and in the right order, or the guard would be
// satisfied by a notification that says the wrong thing.
func TestNotifySendPassesSummaryThenBody(t *testing.T) {
	args := notifySendArgs("The Title", "The Body")
	if len(args) != 3 {
		t.Fatalf("argv = %q, want 3 elements", args)
	}
	if args[0] != "--" {
		t.Errorf("argv[0] = %q, want the `--` terminator", args[0])
	}
	if args[1] != "The Title" {
		t.Errorf("summary = %q, want %q", args[1], "The Title")
	}
	if args[2] != "The Body" {
		t.Errorf("body = %q, want %q", args[2], "The Body")
	}
}

// Awkward but legitimate content must survive untouched — no shell is involved, so
// there is nothing to strip, and mangling it would be a bug of its own.
func TestNotifySendPreservesAwkwardContent(t *testing.T) {
	for _, v := range []string{
		"a & b", "quote's", `double "quote"`, "$(id)", "`id`", "semi;colon",
		"pipe|d", "new\nline", "unicode ✅ 日本語", "", "   ",
	} {
		args := notifySendArgs(v, v)
		if args[1] != v || args[2] != v {
			t.Errorf("content %q was altered: %q", v, args)
		}
	}
}
