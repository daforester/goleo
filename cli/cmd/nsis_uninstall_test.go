package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NSIS writes no Add/Remove Programs entry for you, and goleo was not writing one either
// — so an installed app never appeared in Windows Settings or Control Panel and the only
// way to remove it was finding uninstall.exe in Program Files by hand.
func TestInstallerRegistersWithAddRemovePrograms(t *testing.T) {
	cfg := bundleConfig{AppName: "My App", Version: "1.2.3", Publisher: "Acme Ltd"}
	nsi := nsisScript(cfg, `C:\out\app.exe`, "app.exe", `C:\out\setup.exe`)

	key := `Software\Microsoft\Windows\CurrentVersion\Uninstall\my-app`
	for _, want := range []string{
		`WriteRegStr HKLM "` + key + `" "DisplayName" "My App"`,
		`WriteRegStr HKLM "` + key + `" "DisplayVersion" "1.2.3"`,
		`WriteRegStr HKLM "` + key + `" "Publisher" "Acme Ltd"`,
		// Quoted, or Windows mis-parses a path with spaces — and $PROGRAMFILES64 has one.
		`"UninstallString" "$\"$INSTDIR\uninstall.exe$\""`,
		`"QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"`,
		`"DisplayIcon" "$\"$INSTDIR\app.exe$\""`,
		`WriteRegDWORD HKLM "` + key + `" "NoModify" 1`,
		`WriteRegDWORD HKLM "` + key + `" "NoRepair" 1`,
		// And the uninstaller must remove what the installer wrote.
		`DeleteRegKey HKLM "` + key + `"`,
	} {
		if !strings.Contains(nsi, want) {
			t.Errorf("generated NSIS is missing:\n  %s\n--- script ---\n%s", want, nsi)
		}
	}
}

// Registry paths must NOT have their backslashes doubled.
//
// The first cut used %q for the key, which emitted
// "Software\Microsoft\Windows\..." — and NSIS does not treat backslash as an escape,
// so that is a literal doubled separator. Windows collapses those in FILE paths (which is
// why the existing `File %q` line works despite doing the same thing) but the registry
// does not: the key created is not the key Windows reads for Add/Remove Programs, so the
// entry would silently not appear and DeleteRegKey would not find it.
func TestRegistryPathsAreNotDoubleEscaped(t *testing.T) {
	cfg := bundleConfig{AppName: "My App", Version: "1.2.3"}
	nsi := nsisScript(cfg, `C:\out\app.exe`, "app.exe", `C:\out\setup.exe`)

	for _, line := range strings.Split(nsi, "\n") {
		if !strings.Contains(line, "Reg") {
			continue
		}
		// Two consecutive backslashes. Written as an escaped interpreted string rather
		// than a raw literal: the first version used a raw literal that ended up holding
		// ONE backslash, so it matched every registry path and the test failed always —
		// including, misleadingly, under mutation.
		if strings.Contains(line, "\\\\") {
			t.Errorf("registry line has doubled backslashes, so it targets the wrong key:\n  %s", line)
		}
	}
}

// The self-updater renames the running exe aside as <name>.old, so an app that has ever
// updated itself leaves that file behind and $INSTDIR would survive the uninstall.
func TestUninstallRemovesTheUpdaterBackup(t *testing.T) {
	cfg := bundleConfig{AppName: "My App", Version: "1.2.3"}
	nsi := nsisScript(cfg, `C:\out\app.exe`, "app.exe", `C:\out\setup.exe`)
	if !strings.Contains(nsi, `Delete "$INSTDIR\app.exe.old"`) {
		t.Errorf("uninstall should remove the updater's .old backup:\n%s", nsi)
	}
}

// EstimatedSize is what Settings shows in the size column. It comes from the real binary,
// and must be omitted rather than guessed when the path cannot be read.
func TestEstimatedSizeComesFromTheBinary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "app.exe")
	if err := os.WriteFile(bin, make([]byte, 2048), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := estimatedSizeKB(bin); got != 2 {
		t.Errorf("estimatedSizeKB(2048 bytes) = %d, want 2", got)
	}
	// Rounds up, so a small binary never reports 0 KB.
	if err := os.WriteFile(bin, make([]byte, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := estimatedSizeKB(bin); got != 1 {
		t.Errorf("estimatedSizeKB(1 byte) = %d, want 1", got)
	}
	if got := estimatedSizeKB(filepath.Join(dir, "missing.exe")); got != 0 {
		t.Errorf("a missing binary should yield 0 (key omitted), got %d", got)
	}

	// Match the registry LINE, not the bare word. Checking for "EstimatedSize" anywhere
	// in the script matched the `File "...\TestEstimatedSizeComesFromTheBinary\...` path,
	// because t.TempDir() embeds the test's own name — a false positive that made this
	// assertion pass or fail on what the test happens to be called.
	cfg := bundleConfig{AppName: "My App", Version: "1.2.3"}
	if strings.Contains(nsisScript(cfg, filepath.Join(dir, "missing.exe"), "app.exe", "s.exe"),
		`"EstimatedSize"`) {
		t.Error("EstimatedSize should be omitted when the binary cannot be read")
	}
}
