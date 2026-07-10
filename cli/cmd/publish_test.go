package cmd

import (
	"crypto/ed25519"
	"testing"

	"github.com/daforester/goleo/runtime/updater"
)

func TestMergeAndSign_VerifiesAndAccumulates(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	// First platform.
	win := updater.Release{Version: "1.0.0", URL: "https://x/app-win.exe", SHA256: "aa"}
	doc, err := mergeAndSign(nil, "windows/amd64", win, priv)
	if err != nil {
		t.Fatal(err)
	}
	m, err := updater.VerifyManifest(doc, pub)
	if err != nil {
		t.Fatalf("signed manifest failed to verify: %v", err)
	}
	if m.Releases["windows/amd64"].Version != "1.0.0" {
		t.Fatalf("windows release missing: %+v", m)
	}

	// Second platform merges into the existing manifest without dropping the first.
	mac := updater.Release{Version: "1.0.0", URL: "https://x/app-mac", SHA256: "bb"}
	doc2, err := mergeAndSign(doc, "darwin/arm64", mac, priv)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := updater.VerifyManifest(doc2, pub)
	if err != nil {
		t.Fatal(err)
	}
	if len(m2.Releases) != 2 {
		t.Fatalf("expected 2 platforms after merge, got %d: %+v", len(m2.Releases), m2)
	}
	if m2.Releases["windows/amd64"].SHA256 != "aa" || m2.Releases["darwin/arm64"].SHA256 != "bb" {
		t.Fatalf("merged manifest wrong: %+v", m2)
	}

	// Re-publishing the same platform overwrites its entry.
	winV2 := updater.Release{Version: "2.0.0", URL: "https://x/app-win.exe", SHA256: "cc"}
	doc3, _ := mergeAndSign(doc2, "windows/amd64", winV2, priv)
	m3, _ := updater.VerifyManifest(doc3, pub)
	if m3.Releases["windows/amd64"].Version != "2.0.0" || len(m3.Releases) != 2 {
		t.Fatalf("overwrite failed: %+v", m3)
	}
}
