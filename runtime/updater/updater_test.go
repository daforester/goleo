package updater

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
)

// signManifest is a test helper mirroring what a publisher's signing tool does.
func signManifest(t *testing.T, priv ed25519.PrivateKey, m Manifest) []byte {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, raw)
	doc, err := json.Marshal(signedManifest{
		Manifest:  raw,
		Signature: base64.StdEncoding.EncodeToString(sig),
	})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestVerifyManifest_RoundTripAndTamper(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := Manifest{Releases: map[string]Release{
		"windows/amd64": {Version: "1.2.0", URL: "https://x/app.exe", SHA256: "abc"},
	}}
	doc := signManifest(t, priv, m)

	got, err := VerifyManifest(doc, pub)
	if err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	if got.Releases["windows/amd64"].Version != "1.2.0" {
		t.Fatalf("unexpected manifest: %+v", got)
	}

	// Tampered payload: flip a byte inside the signed manifest JSON.
	tampered := make([]byte, len(doc))
	copy(tampered, doc)
	idx := 20
	tampered[idx] ^= 0xFF
	if _, err := VerifyManifest(tampered, pub); err == nil {
		t.Error("tampered manifest was accepted")
	}

	// Wrong key: a different keypair must not verify.
	otherPub, _, _ := ed25519.GenerateKey(nil)
	if _, err := VerifyManifest(doc, otherPub); err == nil {
		t.Error("manifest verified under the wrong public key")
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.2.0", "1.1.9", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0", "1.9.9", 1},
		{"1.0.0", "1.0", 0},
		{"1.10.0", "1.9.0", 1}, // numeric, not lexical
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCheckForUpdate(t *testing.T) {
	m := &Manifest{Releases: map[string]Release{
		"windows/amd64": {Version: "1.2.0"},
	}}
	if rel, ok := CheckForUpdate("1.1.0", m, "windows/amd64"); !ok || rel.Version != "1.2.0" {
		t.Errorf("expected update available")
	}
	if _, ok := CheckForUpdate("1.2.0", m, "windows/amd64"); ok {
		t.Error("same version should not be an update")
	}
	if _, ok := CheckForUpdate("1.3.0", m, "windows/amd64"); ok {
		t.Error("newer current version should not be an update")
	}
	if _, ok := CheckForUpdate("1.0.0", m, "linux/arm64"); ok {
		t.Error("missing platform should not be an update")
	}
}
