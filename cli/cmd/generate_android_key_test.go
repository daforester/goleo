package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The generated password must never be something keytool can mistake for an option.
//
// The first version used base64.RawURLEncoding, whose alphabet contains "-", and duly
// produced a password starting with one. keytool read it as a flag:
//
//	Warning: The -i-rfbcl8cs79vy12r2_6acqdy8kf2mr option is specified multiple times.
//
// The keystore was still created, with a password that was not the one printed — so the
// command handed over credentials that did not work. keytool takes -storepass as a bare
// argv element with no --flag=value form and no "--" terminator, so the only fix is to
// never generate such a value. Same class as the zenity and notify-send flag confusion
// fixed elsewhere in this repo.
func TestRandomPasswordCannotBeMistakenForAnOption(t *testing.T) {
	// Many samples: the original bug only showed up when a byte happened to encode to a
	// leading dash, so one sample would usually pass.
	for i := 0; i < 500; i++ {
		p, err := randomPassword()
		if err != nil {
			t.Fatal(err)
		}
		if p == "" {
			t.Fatal("empty password")
		}
		if strings.HasPrefix(p, "-") {
			t.Fatalf("password %q starts with a dash — keytool would parse it as an option", p)
		}
		// Nothing needing shell quoting either, since these get pasted into env
		// assignments, .env files and CI secrets.
		for _, c := range p {
			isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
			if !isHex {
				t.Fatalf("password %q contains %q; only [0-9a-f] is safe to pass unquoted", p, c)
			}
		}
		if len(p) < 32 {
			t.Fatalf("password %q is only %d chars — too short for a signing key", p, len(p))
		}
	}
}

// Two calls must not agree, or the "randomness" is decorative.
func TestRandomPasswordIsNotConstant(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p, err := randomPassword()
		if err != nil {
			t.Fatal(err)
		}
		if seen[p] {
			t.Fatalf("randomPassword repeated %q", p)
		}
		seen[p] = true
	}
}

// A user-supplied password must be echoed back as a placeholder, not reprinted — they
// already have it, and reprinting puts it in another scrollback buffer or CI log.
func TestUserSuppliedPasswordIsNotReprinted(t *testing.T) {
	if got := maskUnlessGenerated("s3cret", false); strings.Contains(got, "s3cret") {
		t.Errorf("a user-set password was echoed back: %q", got)
	}
	// A generated one has to be shown: this is the only place it exists.
	if got := maskUnlessGenerated("abc123", true); got != "abc123" {
		t.Errorf("a generated password must be shown, got %q", got)
	}
}

// A keystore path that cannot be opened must fail before the build, not after it.
//
// Reported from real use: `set GOLEO_ANDROID_KEYSTORE=...\test.jks ` (with the trailing
// space cmd.exe keeps) got all the way to :app:packageRelease and then failed with
// "Trailing char < > at index 141" — 37 seconds in, naming neither the variable nor the
// space. Whitespace is now tolerated, and a genuinely bad path is refused up front.
func TestKeystorePathIsValidatedEarly(t *testing.T) {
	origKS, hadKS := os.LookupEnv("GOLEO_ANDROID_KEYSTORE")
	t.Cleanup(func() {
		if hadKS {
			os.Setenv("GOLEO_ANDROID_KEYSTORE", origKS)
		} else {
			os.Unsetenv("GOLEO_ANDROID_KEYSTORE")
		}
		buildRelease, buildNoSign = false, false
	})
	buildRelease, buildNoSign = true, false

	// A path that does not exist is refused, and the message names the path.
	missing := filepath.Join(t.TempDir(), "nope.jks")
	os.Setenv("GOLEO_ANDROID_KEYSTORE", missing)
	err := validateAndroidRelease()
	if err == nil {
		t.Fatal("a nonexistent keystore should be refused before building")
	}
	if !strings.Contains(err.Error(), "nope.jks") {
		t.Errorf("the error should name the path:\n%s", err)
	}

	// A real file passes...
	real := filepath.Join(t.TempDir(), "real.jks")
	if err := os.WriteFile(real, []byte("not really a keystore"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Setenv("GOLEO_ANDROID_KEYSTORE", real)
	if err := validateAndroidRelease(); err != nil {
		t.Errorf("an existing keystore should validate: %v", err)
	}

	// ...and so does the same path with the whitespace cmd.exe leaves behind, because
	// tolerating it is better than failing on it.
	os.Setenv("GOLEO_ANDROID_KEYSTORE", "  "+real+"  ")
	if err := validateAndroidRelease(); err != nil {
		t.Errorf("a padded path should be trimmed and accepted, got: %v", err)
	}
}

// Gradle reads these variables itself, so what it receives must already be clean.
func TestAndroidSigningEnvIsTrimmed(t *testing.T) {
	for _, k := range []string{
		"GOLEO_ANDROID_KEYSTORE", "GOLEO_ANDROID_KEYSTORE_PASSWORD",
		"GOLEO_ANDROID_KEY_ALIAS", "GOLEO_ANDROID_KEY_PASSWORD",
	} {
		orig, had := os.LookupEnv(k)
		t.Cleanup(func() {
			if had {
				os.Setenv(k, orig)
			} else {
				os.Unsetenv(k)
			}
		})
		os.Setenv(k, "  padded  ")
	}

	env := androidSigningEnv()
	if len(env) != 4 {
		t.Fatalf("expected all four variables, got %v", env)
	}
	for _, kv := range env {
		if !strings.HasSuffix(kv, "=padded") {
			t.Errorf("%q was not trimmed — Gradle would receive the whitespace", kv)
		}
	}

	// An unset variable must not be exported as empty: that would blank a value the
	// developer had set in a parent environment.
	os.Unsetenv("GOLEO_ANDROID_KEY_ALIAS")
	for _, kv := range androidSigningEnv() {
		if strings.HasPrefix(kv, "GOLEO_ANDROID_KEY_ALIAS=") {
			t.Errorf("an unset variable should not be exported at all, got %q", kv)
		}
	}
}
