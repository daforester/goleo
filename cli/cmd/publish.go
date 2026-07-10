package cmd

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/daforester/goleo/runtime/updater"
)

// runPublish copies the built binary into dist/bundle/ under a platform-specific
// name, computes its SHA256, and merges a Release for the current platform into
// an ed25519-signed update manifest (dist/bundle/manifest.json). The app's
// updater client (runtime/updater) consumes this manifest. Repeated runs on
// different platforms accumulate into one manifest.
func runPublish(target buildTarget, binaryPath string, cfg bundleConfig) error {
	priv, err := loadUpdatePrivKey()
	if err != nil {
		return err
	}
	if cfg.UpdateURLBase == "" {
		return fmt.Errorf("publish: set \"bundle.update_url_base\" in goleo.json (where update artifacts are hosted)")
	}

	outDir := filepath.Join("dist", "bundle")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	artifact := fmt.Sprintf("%s-%s-%s-%s%s", slug(cfg.AppName), cfg.Version, target.GOOS, target.GOARCH, target.OutputExt)
	dst := filepath.Join(outDir, artifact)
	if err := copyFile(binaryPath, dst); err != nil {
		return fmt.Errorf("publish: staging artifact: %w", err)
	}
	sum, err := sha256File(dst)
	if err != nil {
		return err
	}

	rel := updater.Release{
		Version: cfg.Version,
		URL:     strings.TrimRight(cfg.UpdateURLBase, "/") + "/" + artifact,
		SHA256:  sum,
		Notes:   cfg.ReleaseNotes,
	}
	platform := target.GOOS + "/" + target.GOARCH

	manifestPath := filepath.Join(outDir, "manifest.json")
	existing, _ := os.ReadFile(manifestPath)
	signed, err := mergeAndSign(existing, platform, rel, priv)
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, signed, 0o644); err != nil {
		return err
	}

	fmt.Printf("\n  Published %s → %s\n", platform, rel.URL)
	fmt.Printf("  Signed manifest: %s\n", manifestPath)
	return nil
}

// mergeAndSign updates (or creates) the manifest with rel for platform and
// re-signs it. It reads the inner manifest from an existing signed document
// without verifying (the publisher holds the key and re-signs anyway).
func mergeAndSign(existing []byte, platform string, rel updater.Release, priv ed25519.PrivateKey) ([]byte, error) {
	m := updater.Manifest{Releases: map[string]updater.Release{}}
	if len(existing) > 0 {
		var doc struct {
			Manifest json.RawMessage `json:"manifest"`
		}
		if json.Unmarshal(existing, &doc) == nil && len(doc.Manifest) > 0 {
			var em updater.Manifest
			if json.Unmarshal(doc.Manifest, &em) == nil && em.Releases != nil {
				m = em
			}
		}
	}
	m.Releases[platform] = rel
	return updater.SignManifest(&m, priv)
}

func loadUpdatePrivKey() (ed25519.PrivateKey, error) {
	b64 := os.Getenv("GOLEO_UPDATE_PRIVKEY")
	if b64 == "" {
		return nil, fmt.Errorf("publish: set GOLEO_UPDATE_PRIVKEY (base64 ed25519 private key) — generate one with `goleo generate updater-key`")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("publish: decoding GOLEO_UPDATE_PRIVKEY: %w", err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("publish: GOLEO_UPDATE_PRIVKEY is not a valid ed25519 private key")
	}
	return ed25519.PrivateKey(raw), nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
