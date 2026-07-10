package cmd

import "testing"

func TestSignConfig_DisabledByDefault(t *testing.T) {
	// With the relevant env vars unset, every signing mode is disabled so a
	// local `--bundle` produces unsigned artifacts (with a notice) rather than
	// failing.
	for _, k := range []string{
		"GOLEO_WIN_CERT", "GOLEO_WIN_CERT_PASSWORD",
		"GOLEO_MAC_IDENTITY", "GOLEO_APPLE_ID", "GOLEO_APPLE_TEAM_ID", "GOLEO_APPLE_PASSWORD",
	} {
		t.Setenv(k, "")
	}
	sc := loadSignConfig()
	if sc.windowsEnabled() || sc.macSignEnabled() || sc.macNotarizeEnabled() {
		t.Errorf("expected all signing disabled when env unset, got %+v", sc)
	}
	if sc.winTimestampURL == "" {
		t.Error("timestamp URL should default when unset")
	}
}

func TestSignConfig_EnabledFromEnv(t *testing.T) {
	t.Setenv("GOLEO_WIN_CERT", "cert.pfx")
	t.Setenv("GOLEO_MAC_IDENTITY", "Developer ID Application: Acme")
	t.Setenv("GOLEO_APPLE_ID", "dev@acme.com")
	t.Setenv("GOLEO_APPLE_TEAM_ID", "ABCDE12345")
	t.Setenv("GOLEO_APPLE_PASSWORD", "app-specific-pw")

	sc := loadSignConfig()
	if !sc.windowsEnabled() {
		t.Error("windows signing should be enabled")
	}
	if !sc.macSignEnabled() {
		t.Error("mac signing should be enabled")
	}
	if !sc.macNotarizeEnabled() {
		t.Error("mac notarization should be enabled")
	}
}

func TestSignConfig_PartialNotarizeDisabled(t *testing.T) {
	t.Setenv("GOLEO_APPLE_ID", "dev@acme.com")
	t.Setenv("GOLEO_APPLE_TEAM_ID", "")
	t.Setenv("GOLEO_APPLE_PASSWORD", "")
	if loadSignConfig().macNotarizeEnabled() {
		t.Error("notarization needs all three creds; partial should be disabled")
	}
}
