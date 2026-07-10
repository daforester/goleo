package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// signConfig holds code-signing credentials, sourced from environment variables
// so secrets never live in the repo and CI can inject them. When the relevant
// vars are unset, signing is skipped with a notice rather than failing — so an
// unsigned local `--bundle` still works.
type signConfig struct {
	// Windows Authenticode
	winCertFile     string // GOLEO_WIN_CERT (path to .pfx/.p12)
	winCertPass     string // GOLEO_WIN_CERT_PASSWORD
	winTimestampURL string // GOLEO_WIN_TIMESTAMP (default DigiCert)
	// macOS codesign + notarization
	macIdentity   string // GOLEO_MAC_IDENTITY ("Developer ID Application: …")
	appleID       string // GOLEO_APPLE_ID
	teamID        string // GOLEO_APPLE_TEAM_ID
	applePassword string // GOLEO_APPLE_PASSWORD (app-specific password)
}

func loadSignConfig() signConfig {
	ts := os.Getenv("GOLEO_WIN_TIMESTAMP")
	if ts == "" {
		ts = "http://timestamp.digicert.com"
	}
	return signConfig{
		winCertFile:     os.Getenv("GOLEO_WIN_CERT"),
		winCertPass:     os.Getenv("GOLEO_WIN_CERT_PASSWORD"),
		winTimestampURL: ts,
		macIdentity:     os.Getenv("GOLEO_MAC_IDENTITY"),
		appleID:         os.Getenv("GOLEO_APPLE_ID"),
		teamID:          os.Getenv("GOLEO_APPLE_TEAM_ID"),
		applePassword:   os.Getenv("GOLEO_APPLE_PASSWORD"),
	}
}

func (s signConfig) windowsEnabled() bool { return s.winCertFile != "" }
func (s signConfig) macSignEnabled() bool { return s.macIdentity != "" }
func (s signConfig) macNotarizeEnabled() bool {
	return s.appleID != "" && s.teamID != "" && s.applePassword != ""
}

// signWindows Authenticode-signs a file (app binary or installer) with a
// timestamped SHA-256 signature. No-op (with notice) when unconfigured.
func signWindows(sc signConfig, file string) error {
	if !sc.windowsEnabled() {
		fmt.Printf("  Signing skipped for %s (set GOLEO_WIN_CERT + GOLEO_WIN_CERT_PASSWORD to enable)\n", filepath.Base(file))
		return nil
	}
	tool, err := requireTool("signtool", "part of the Windows SDK")
	if err != nil {
		return err
	}
	args := []string{"sign", "/fd", "sha256", "/f", sc.winCertFile}
	if sc.winCertPass != "" {
		args = append(args, "/p", sc.winCertPass)
	}
	args = append(args, "/tr", sc.winTimestampURL, "/td", "sha256", file)
	cmd := exec.Command(tool, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bundle: signtool failed: %w", err)
	}
	fmt.Printf("  Signed %s\n", filepath.Base(file))
	return nil
}

// codesignMac signs a .app bundle (deep, hardened runtime). No-op when
// unconfigured.
func codesignMac(sc signConfig, path string) error {
	if !sc.macSignEnabled() {
		fmt.Printf("  Code signing skipped for %s (set GOLEO_MAC_IDENTITY to enable)\n", filepath.Base(path))
		return nil
	}
	tool, err := requireTool("codesign", "part of the Xcode command line tools")
	if err != nil {
		return err
	}
	cmd := exec.Command(tool, "--deep", "--force", "--options", "runtime", "--timestamp", "--sign", sc.macIdentity, path)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bundle: codesign failed: %w", err)
	}
	fmt.Printf("  Code-signed %s\n", filepath.Base(path))
	return nil
}

// notarizeMac submits a .dmg to Apple's notary service, waits, and staples the
// ticket. No-op when unconfigured.
func notarizeMac(sc signConfig, dmg string) error {
	if !sc.macNotarizeEnabled() {
		fmt.Printf("  Notarization skipped for %s (set GOLEO_APPLE_ID + GOLEO_APPLE_TEAM_ID + GOLEO_APPLE_PASSWORD to enable)\n", filepath.Base(dmg))
		return nil
	}
	tool, err := requireTool("xcrun", "part of the Xcode command line tools")
	if err != nil {
		return err
	}
	submit := exec.Command(tool, "notarytool", "submit", dmg,
		"--apple-id", sc.appleID, "--team-id", sc.teamID, "--password", sc.applePassword, "--wait")
	submit.Stdout, submit.Stderr = os.Stdout, os.Stderr
	if err := submit.Run(); err != nil {
		return fmt.Errorf("bundle: notarytool failed: %w", err)
	}
	staple := exec.Command(tool, "stapler", "staple", dmg)
	staple.Stdout, staple.Stderr = os.Stdout, os.Stderr
	if err := staple.Run(); err != nil {
		return fmt.Errorf("bundle: stapler failed: %w", err)
	}
	fmt.Printf("  Notarized + stapled %s\n", filepath.Base(dmg))
	return nil
}
