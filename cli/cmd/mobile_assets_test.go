package cmd

import (
	"io/fs"
	"strings"
	"testing"
	"text/template"
	"unicode/utf8"
)

// The frontend is embedded in the Go library and served over http://127.0.0.1:<port>; the
// native shells load that URL. So the build must NOT also copy frontend/dist into the
// native project — it did, on both platforms, shipping a second copy of the whole frontend
// in every APK, AAB and .app that nothing ever read.
//
// This test is the other half of that removal: if a shell ever starts loading from native
// assets, it will find nothing there, so the reference and the copy have to come back
// together. Failing here says which one is missing.
func TestNativeShellsDoNotLoadFromBundledAssets(t *testing.T) {
	// Patterns that mean "read the UI out of the native bundle rather than over loopback".
	forbidden := map[string]string{
		"file:///android_asset": "Android asset URL",
		"getAssets(":            "Android AssetManager",
		"AssetManager":          "Android AssetManager",
		"loadFileURL":           "WKWebView file URL load",
		"Bundle.main.url":       "iOS bundle resource lookup",
		"Bundle.main.path":      "iOS bundle resource lookup",
	}

	roots := []string{"templates/android", "templates/android-dev", "templates/ios"}
	for _, root := range roots {
		err := fs.WalkDir(mobileTemplates, root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			switch {
			case strings.HasSuffix(path, ".java"),
				strings.HasSuffix(path, ".kt"),
				strings.HasSuffix(path, ".kts"),
				strings.HasSuffix(path, ".swift"):
			default:
				return nil
			}
			b, err := mobileTemplates.ReadFile(path)
			if err != nil {
				return err
			}
			for pattern, what := range forbidden {
				if strings.Contains(string(b), pattern) {
					t.Errorf("%s references %s (%q), but the build no longer copies "+
						"frontend/dist into the native project — either restore the copy or "+
						"load over http://127.0.0.1:<port> as the other shells do",
						path, what, pattern)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
}

// And the shells must load over loopback, so the embedded server is genuinely the source
// of the UI rather than this being a copy nobody noticed was unused.
func TestNativeShellsLoadOverLoopback(t *testing.T) {
	for _, path := range []string{
		"templates/android/app/src/main/java/com/goleo/app/MainActivity.java",
		"templates/ios/App/AppDelegate.swift",
	} {
		b, err := mobileTemplates.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if !strings.Contains(string(b), "127.0.0.1") && !strings.Contains(string(b), "localhost") {
			t.Errorf("%s does not load over a loopback URL; if the UI now comes from "+
				"somewhere else, the removed frontend/dist copy may be needed again", path)
		}
	}
}

// Every mobile template must PARSE as a text/template.
//
// extractMobileTemplate runs each file through text/template, so a stray `{{...}}` anywhere —
// including inside a YAML or XML comment, where it looks inert — is a real action. A comment
// added to xcodegen.yml explaining the HasIcon conditional contained a literal `{{if
// .HasIcon}}`, which opened an unterminated if and broke the whole file with
// "unexpected EOF". Nothing outside a real iOS build would have caught that.
func TestEveryMobileTemplateParses(t *testing.T) {
	roots := []string{"templates/android", "templates/android-dev", "templates/ios"}
	checked := 0
	for _, root := range roots {
		err := fs.WalkDir(mobileTemplates, root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			raw, rerr := mobileTemplates.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			// Binary files (the vendored wrapper jar, PNGs) are copied, not templated.
			if !utf8.Valid(raw) {
				return nil
			}
			if _, perr := template.New(path).Parse(string(raw)); perr != nil {
				t.Errorf("%s does not parse as a template: %v\n"+
					"  (a `{{...}}` in a comment is still an action)", path, perr)
			}
			checked++
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if checked < 10 {
		t.Errorf("only %d templates checked — the walk is not finding them", checked)
	}
	t.Logf("parsed %d mobile templates", checked)
}

// The scaffolded .gitignore must exclude signing keys.
//
// `goleo generate android-key` writes release.jks INTO the project directory and prints
// "do not commit them" — advice, with nothing enforcing it. A committed upload key lets
// anyone sign updates as you, and Play will not let a listing change the key it uses, so
// this is not recoverable by rotating.
//
// Found because a shell mistake ran that command in the repo root and left an untracked,
// un-ignored keystore that `git add -A` would have committed.
func TestScaffoldedGitignoreExcludesSigningKeys(t *testing.T) {
	for _, pattern := range []string{"*.jks", "*.keystore", "*.p12", "*.mobileprovision"} {
		if !strings.Contains(tmplGitignore, pattern) {
			t.Errorf("the scaffolded .gitignore does not exclude %s — `goleo generate "+
				"android-key` writes a keystore into the project, so an accidental commit is "+
				"one `git add -A` away", pattern)
		}
	}
	// The .gitignore is emitted inside a Go RAW string literal, so a backtick in it
	// terminates the literal and breaks the build. That happened while adding the block above.
	if strings.Contains(tmplGitignore, "`") {
		t.Error("tmplGitignore contains a backtick, which terminates the raw string literal " +
			"it lives in")
	}
}
