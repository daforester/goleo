// Package updater is a desktop auto-update client. A publisher signs a release
// manifest with an ed25519 private key; the app embeds the matching public key
// and verifies the manifest before trusting any release info or downloading an
// artifact. This signature scheme is independent of OS code-signing — it
// authenticates the *update metadata* so a tampered or spoofed manifest is
// rejected. Pure Go; the security-critical parts (verify, version compare) are
// unit-tested. Self-replacement/relaunch is desktop-only.
package updater

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Release describes a downloadable build for one platform.
type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"` // hex digest of the artifact
	Notes   string `json:"notes,omitempty"`
}

// Manifest maps a platform key ("<GOOS>/<GOARCH>") to its latest Release.
type Manifest struct {
	Releases map[string]Release `json:"releases"`
}

// signedManifest is the on-the-wire format: the raw manifest JSON plus a
// base64 ed25519 signature over exactly those bytes.
type signedManifest struct {
	Manifest  json.RawMessage `json:"manifest"`
	Signature string          `json:"signature"`
}

// Config configures the update client.
type Config struct {
	ManifestURL    string // URL of the signed manifest JSON
	PublicKey      string // base64-encoded ed25519 public key
	CurrentVersion string // the running app's version
}

// PlatformKey is the manifest key for the current build.
func PlatformKey() string { return runtime.GOOS + "/" + runtime.GOARCH }

// VerifyManifest parses a signed-manifest document and verifies its ed25519
// signature against pubKey, returning the inner Manifest only if valid.
func VerifyManifest(data []byte, pubKey ed25519.PublicKey) (*Manifest, error) {
	var sm signedManifest
	if err := json.Unmarshal(data, &sm); err != nil {
		return nil, fmt.Errorf("updater: parsing signed manifest: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(sm.Signature)
	if err != nil {
		return nil, fmt.Errorf("updater: decoding signature: %w", err)
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return nil, errors.New("updater: invalid public key size")
	}
	if !ed25519.Verify(pubKey, sm.Manifest, sig) {
		return nil, errors.New("updater: manifest signature verification failed")
	}
	var m Manifest
	if err := json.Unmarshal(sm.Manifest, &m); err != nil {
		return nil, fmt.Errorf("updater: parsing manifest: %w", err)
	}
	return &m, nil
}

// SignManifest produces a signed-manifest document: the manifest JSON plus a
// base64 ed25519 signature over exactly those bytes. It is the counterpart to
// VerifyManifest and is used by the publisher (goleo build --publish).
func SignManifest(m *Manifest, priv ed25519.PrivateKey) ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	sig := ed25519.Sign(priv, raw)
	return json.Marshal(signedManifest{
		Manifest:  raw,
		Signature: base64.StdEncoding.EncodeToString(sig),
	})
}

// CheckForUpdate returns the release for platform if it is newer than current.
func CheckForUpdate(current string, m *Manifest, platform string) (*Release, bool) {
	rel, ok := m.Releases[platform]
	if !ok {
		return nil, false
	}
	if compareVersions(rel.Version, current) <= 0 {
		return nil, false
	}
	r := rel
	return &r, true
}

// compareVersions compares dotted numeric versions (optional leading "v"),
// returning -1, 0, or 1. Non-numeric segments compare as 0.
func compareVersions(a, b string) int {
	as := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bs := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var ai, bi int
		if i < len(as) {
			ai, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bi, _ = strconv.Atoi(bs[i])
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Client checks for and downloads updates.
type Client struct {
	cfg    Config
	pubKey ed25519.PublicKey
	http   *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	pk, err := base64.StdEncoding.DecodeString(cfg.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("updater: decoding public key: %w", err)
	}
	return &Client{cfg: cfg, pubKey: ed25519.PublicKey(pk), http: http.DefaultClient}, nil
}

// Check fetches and verifies the manifest and returns the newer release for the
// current platform, or nil if the app is up to date.
func (c *Client) Check() (*Release, error) {
	resp, err := c.http.Get(c.cfg.ManifestURL)
	if err != nil {
		return nil, fmt.Errorf("updater: fetching manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("updater: manifest HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("updater: reading manifest: %w", err)
	}
	m, err := VerifyManifest(data, c.pubKey)
	if err != nil {
		return nil, err
	}
	rel, ok := CheckForUpdate(c.cfg.CurrentVersion, m, PlatformKey())
	if !ok {
		return nil, nil
	}
	return rel, nil
}

// Download fetches the release artifact to a temp file and verifies its SHA256.
// progress, if non-nil, is called with (bytesDownloaded, totalBytes).
func (c *Client) Download(rel *Release, progress func(done, total int64)) (string, error) {
	resp, err := c.http.Get(rel.URL)
	if err != nil {
		return "", fmt.Errorf("updater: downloading artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("updater: artifact HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "goleo-update-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	h := sha256.New()
	total := resp.ContentLength
	var done int64
	buf := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := tmp.Write(buf[:n]); werr != nil {
				os.Remove(tmp.Name())
				return "", werr
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			os.Remove(tmp.Name())
			return "", rerr
		}
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, rel.SHA256) {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("updater: artifact SHA256 mismatch (got %s, want %s)", got, rel.SHA256)
	}
	return tmp.Name(), nil
}

// ApplyAndRelaunch replaces the running executable with the downloaded artifact
// and relaunches it. Desktop-only. On Windows a running .exe cannot be
// overwritten in place, so the current binary is moved aside first.
//
// NOTE: self-replacement is OS-specific and must be validated by running a
// real packaged app; the verify/download path above is the security-critical,
// unit-tested part.
func ApplyAndRelaunch(newBinary string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)

	if runtime.GOOS == "windows" {
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return fmt.Errorf("updater: moving current binary aside: %w", err)
		}
		if err := os.Rename(newBinary, exe); err != nil {
			os.Rename(old, exe) // best-effort rollback
			return fmt.Errorf("updater: installing new binary: %w", err)
		}
	} else {
		if err := os.Chmod(newBinary, 0o755); err != nil {
			return err
		}
		if err := os.Rename(newBinary, exe); err != nil {
			return fmt.Errorf("updater: replacing binary: %w", err)
		}
	}
	return relaunch(exe)
}

// relaunch starts a fresh copy of the (now updated) executable with the same
// args and exits the current process.
func relaunch(exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("updater: relaunch: %w", err)
	}
	os.Exit(0)
	return nil
}
