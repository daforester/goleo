package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var (
	androidKeyOut      string
	androidKeyAlias    string
	androidKeyValidity int
	androidKeyDName    string
)

// `goleo generate android-key` wraps keytool.
//
// It exists because `goleo build android --release` refuses without a keystore and told
// people to run keytool — which ships inside the JDK and is usually NOT on PATH. That is
// a silly thing to make someone hunt for, because goleo has already had to locate a JDK
// to run Gradle: resolveJava searches JAVA_HOME, PATH and the usual install locations,
// and installs one if needed. Using that same JDK means the first wall a developer hits
// on the way to shipping is not "keytool: command not found".
//
// It deliberately does not reimplement key generation in Go. keytool produces the exact
// JKS/PKCS12 artifact Gradle's signingConfig expects, and a hand-rolled equivalent would
// be a subtly different thing to debug.
var generateAndroidKeyCmd = &cobra.Command{
	Use:   "android-key",
	Short: "Generate an Android signing keystore for release builds",
	Long: `Generate a keystore for signing Android release builds.

Uses keytool from the JDK goleo already resolves for Gradle, so it works without
keytool being on your PATH.

Passwords come from the environment when set, which is also what the build reads:

  GOLEO_ANDROID_KEYSTORE_PASSWORD   the keystore password
  GOLEO_ANDROID_KEY_PASSWORD        the key password (defaults to the keystore one)

If neither is set, a random password is generated and printed once — fine for a
throwaway test key, but for a real upload key set your own so it is never echoed to a
terminal or a CI log.

Usage:
  goleo generate android-key
  goleo generate android-key --out upload.jks --alias upload --validity 10000`,
	RunE: runGenerateAndroidKey,
}

func init() {
	generateAndroidKeyCmd.Flags().StringVar(&androidKeyOut, "out", "release.jks", "Keystore file to create")
	generateAndroidKeyCmd.Flags().StringVar(&androidKeyAlias, "alias", "upload", "Key alias")
	generateAndroidKeyCmd.Flags().IntVar(&androidKeyValidity, "validity", 10000, "Validity in days (Play requires a key valid past 2033)")
	generateAndroidKeyCmd.Flags().StringVar(&androidKeyDName, "dname", "", "X.500 distinguished name (default: a placeholder — it is not shown to users)")
	generateCmd.AddCommand(generateAndroidKeyCmd)
}

func runGenerateAndroidKey(cmd *cobra.Command, args []string) error {
	// Never silently overwrite: a keystore is unrecoverable, and replacing the one an
	// existing Play listing was signed with means never shipping an update to it again.
	if _, err := os.Stat(androidKeyOut); err == nil {
		return fmt.Errorf("%s already exists — refusing to overwrite a keystore\n"+
			"  If you have lost its password, generate a new one under a different --out;\n"+
			"  a keystore cannot be recovered, and replacing the key an existing Play\n"+
			"  listing uses means you can never update that listing again.", androidKeyOut)
	}

	keytool, err := resolveKeytool()
	if err != nil {
		return err
	}

	storePass := strings.TrimSpace(os.Getenv("GOLEO_ANDROID_KEYSTORE_PASSWORD"))
	keyPass := strings.TrimSpace(os.Getenv("GOLEO_ANDROID_KEY_PASSWORD"))
	generated := false
	if storePass == "" {
		storePass, err = randomPassword()
		if err != nil {
			return err
		}
		generated = true
	}
	if keyPass == "" {
		keyPass = storePass
	}

	dname := androidKeyDName
	if dname == "" {
		// keytool requires a DN. It is not shown to users — Play identifies an app by
		// its package name and signing certificate, not this text — so a placeholder is
		// honest rather than prompting for details that do not matter.
		dname = "CN=goleo, OU=goleo, O=goleo, L=unspecified, ST=unspecified, C=ZZ"
	}

	fmt.Printf("  Generating %s with keytool from %s\n", androidKeyOut, filepath.Dir(keytool))
	kt := exec.Command(keytool,
		"-genkeypair",
		"-keystore", androidKeyOut,
		"-alias", androidKeyAlias,
		"-keyalg", "RSA",
		"-keysize", "2048",
		"-validity", fmt.Sprint(androidKeyValidity),
		"-storepass", storePass,
		"-keypass", keyPass,
		"-dname", dname,
	)
	kt.Stderr = os.Stderr
	if out, err := kt.Output(); err != nil {
		return fmt.Errorf("keytool failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if _, err := os.Stat(androidKeyOut); err != nil {
		return fmt.Errorf("keytool reported success but %s does not exist: %w", androidKeyOut, err)
	}

	// Prove the credentials work before reporting them.
	if err := verifyKeystore(keytool, androidKeyOut, storePass, androidKeyAlias); err != nil {
		os.Remove(androidKeyOut)
		return err
	}

	abs, _ := filepath.Abs(androidKeyOut)
	fmt.Printf("  Created %s (alias %q, valid %d days)\n", androidKeyOut, androidKeyAlias, androidKeyValidity)
	fmt.Println()
	if generated {
		fmt.Println("  A password was generated. It is shown ONCE — save it now:")
		fmt.Println("    " + storePass)
		fmt.Println()
		fmt.Println("  For a real upload key, set GOLEO_ANDROID_KEYSTORE_PASSWORD yourself instead,")
		fmt.Println("  so it is never printed to a terminal or a CI log.")
		fmt.Println()
	}
	fmt.Println("  Set these before `goleo build android --release`:")
	if isWindowsHost() {
		fmt.Printf("    set GOLEO_ANDROID_KEYSTORE=%s\n", abs)
		fmt.Printf("    set GOLEO_ANDROID_KEYSTORE_PASSWORD=%s\n", maskUnlessGenerated(storePass, generated))
		fmt.Printf("    set GOLEO_ANDROID_KEY_ALIAS=%s\n", androidKeyAlias)
		fmt.Printf("    set GOLEO_ANDROID_KEY_PASSWORD=%s\n", maskUnlessGenerated(keyPass, generated))
	} else {
		fmt.Printf("    export GOLEO_ANDROID_KEYSTORE=%s\n", abs)
		fmt.Printf("    export GOLEO_ANDROID_KEYSTORE_PASSWORD=%s\n", maskUnlessGenerated(storePass, generated))
		fmt.Printf("    export GOLEO_ANDROID_KEY_ALIAS=%s\n", androidKeyAlias)
		fmt.Printf("    export GOLEO_ANDROID_KEY_PASSWORD=%s\n", maskUnlessGenerated(keyPass, generated))
	}
	fmt.Println()
	fmt.Println("  Keep the keystore and its password safe, and do not commit them. Losing the")
	fmt.Println("  key used by a published Play listing means you can never update that listing.")
	return nil
}

// maskUnlessGenerated avoids re-printing a password the user already knows and set
// themselves. A generated one has to be echoed since this is the only place it exists.
func maskUnlessGenerated(pass string, generated bool) string {
	if generated {
		return pass
	}
	return "<the value you set>"
}

// resolveKeytool finds keytool in the JDK goleo would use for Gradle.
//
// PATH is checked first so an explicitly installed JDK wins, then the resolved
// JAVA_HOME. Both matter: keytool lives in the JDK's bin/ and is very often absent from
// PATH even when JAVA_HOME is correctly set — which is precisely the situation this
// command exists for.
func resolveKeytool() (string, error) {
	name := "keytool"
	if isWindowsHost() {
		name = "keytool.exe"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}

	var candidates []string
	if jh := strings.TrimSpace(os.Getenv("JAVA_HOME")); jh != "" {
		candidates = append(candidates, filepath.Join(jh, "bin", name))
	}
	// Fall back to the same resolution Gradle builds rely on, so this works wherever
	// `goleo build android` does.
	deps := &androidDeps{}
	if err := deps.resolveJava(); err == nil && deps.JavaHome != "" {
		candidates = append(candidates, filepath.Join(deps.JavaHome, "bin", name))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not find keytool.\n" +
		"  It ships inside the JDK, in its bin/ directory, and is often not on PATH.\n" +
		"  Set JAVA_HOME to a JDK (not a JRE) and retry, or run `goleo doctor android`\n" +
		"  to have goleo resolve or install one.")
}

// randomPassword returns a random hex password.
//
// HEX specifically, not base64. The first version used base64.RawURLEncoding, whose
// alphabet includes "-", and it duly generated a password starting with one — which
// keytool parsed as an OPTION rather than as the value of -storepass:
//
//	Warning: The -i-rfbcl8cs79vy12r2_6acqdy8kf2mr option is specified multiple times.
//
// The keystore was still created, with a password that was not the one printed. That is
// the same flag-confusion class as the zenity and notify-send fixes elsewhere in this
// repo, reintroduced by generating a value that can begin with a dash. keytool takes
// -storepass as a separate argv element with no "--flag=value" form and no "--"
// terminator, so the only reliable fix is to never produce such a value.
//
// [0-9a-f] cannot collide with an option, needs no shell quoting, and survives being
// pasted into a .env file or a CI secret. 24 bytes is 192 bits.
func randomPassword() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating a password: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// verifyKeystore opens the keystore with the password that was just used, so a mangled
// credential fails here rather than at the developer's next release build.
//
// This exists because the dash bug above produced a keystore whose password was not the
// one reported, and nothing noticed: the file was created, keytool exited 0, and the
// command printed instructions that would not have worked. Creating the artifact is not
// the same as creating a usable one.
func verifyKeystore(keytool, path, storePass, alias string) error {
	cmd := exec.Command(keytool, "-list",
		"-keystore", path,
		"-storepass", storePass,
		"-alias", alias,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("the keystore was created but does not open with the password "+
			"reported above — refusing to hand over credentials that do not work: %w\n%s",
			err, strings.TrimSpace(string(out)))
	}
	return nil
}

// isWindowsHost is the host check used for the shell syntax in the printed hints and for
// the keytool executable name.
func isWindowsHost() bool { return runtime.GOOS == "windows" }
