package cmd

import (
	"strings"
	"testing"
)

// mobile.android.package_name is interpolated into `package X;` in generated Java, so it
// must be a valid Java package — not merely a valid Android applicationId. Nothing
// validated it, so a bad value failed inside javac or the manifest merger, naming a
// generated file under .goleo/android instead of the goleo.json line responsible. It also
// deserves an early check because the name is PERMANENT once an artifact reaches Play.
func TestValidAndroidPackageNamesAreAccepted(t *testing.T) {
	for _, ok := range []string{
		"com.example.myapp",
		"com.goleo.app",
		"io.github.someone.app2",
		"a.b",                 // minimal: two segments
		"com.example.my_app",  // underscore is fine
		"com.example.app3x",   // digits after the first char are fine
		"xkqjfhwz.zprtnvbmqd", // random letters, no domain needed
	} {
		if err := validateAndroidPackageName(ok); err != nil {
			t.Errorf("%q should be accepted: %v", ok, err)
		}
	}
}

func TestInvalidAndroidPackageNamesAreRejected(t *testing.T) {
	cases := map[string]string{
		"":                   "empty",
		"myapp":              "two",      // single segment
		"com.example.":       "segment",  // trailing dot
		".com.example":       "segment",  // leading dot
		"com..example":       "segment",  // doubled dot
		"com.example.1app":   "letter",   // segment starts with a digit
		"com.example._app":   "letter",   // segment starts with underscore
		"com.example.my-app": "letters",  // hyphen: legal in a domain, not a package
		"com.example.my app": "letters",  // space
		"com.example.new":    "reserved", // Java keyword
		"com.class.app":      "reserved", // Java keyword mid-name
		"com.example.int":    "reserved", // Java keyword
		" com.example.myapp": "whitespace",
		"com.example.myapp ": "whitespace",
	}
	for bad, wantIn := range cases {
		err := validateAndroidPackageName(bad)
		if err == nil {
			t.Errorf("%q should be rejected", bad)
			continue
		}
		if !strings.Contains(err.Error(), wantIn) {
			t.Errorf("%q: error should mention %q, got: %v", bad, wantIn, err)
		}
	}
}

// The hyphen case is worth calling out separately: it is the mistake people actually make,
// because it is legal in the domain the convention is borrowed from.
func TestHyphenRejectionExplainsItself(t *testing.T) {
	err := validateAndroidPackageName("com.my-company.app")
	if err == nil {
		t.Fatal("a hyphenated segment should be rejected")
	}
	if !strings.Contains(err.Error(), "hyphen") {
		t.Errorf("the error should name the hyphen specifically:\n%v", err)
	}
}
