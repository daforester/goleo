package runtime

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Binary file I/O was broken in BOTH directions.
//
//	read:  the handler returned map[string]string{"data": string(bytes)}, so
//	       arbitrary binary went through encoding/json, which replaces every
//	       invalid UTF-8 sequence with U+FFFD. The payload was already corrupt on
//	       the wire; the TS side then ran TextEncoder over it, re-expanding
//	       multibyte runes, and the caller got garbage of the wrong length.
//	write: the handler declares Data []byte, which encoding/json decodes from
//	       base64 — but the TS side sent TextDecoder output, so a non-ASCII payload
//	       either failed to unmarshal or produced the wrong bytes.
//
// Both ends now use base64, which is what encoding/json already does for []byte.
func TestBinaryRoundTripPreservesExactBytes(t *testing.T) {
	app := New(Config{Title: "Bin", AppID: "bin-test"})
	b := app.Bridge()
	RegisterFS(b)

	// Marshal the args rather than splicing JSON by hand: a Windows path is full of
	// backslashes and hand-escaping them is its own source of bugs.
	args := func(v any) []byte {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	dirResp := b.HandleRequest(InvokeRequest{
		ID: "1", Method: "goleo:fsAppDataDir",
		Args: args(map[string]string{"appName": "bin-test"}),
	})
	if dirResp.Error != "" {
		t.Fatalf("appDataDir: %s", dirResp.Error)
	}
	dir, ok := dirResp.Result.(string)
	if !ok {
		t.Fatalf("appDataDir returned %#v", dirResp.Result)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	target := filepath.Join(dir, "blob.bin")

	// A PNG header followed by bytes that are not valid UTF-8 — precisely what the
	// old path destroyed.
	original := []byte{
		0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A,
		0xFF, 0xFE, 0xFD, 0x00, 0x01, 0x80, 0xC0, 0xC1,
		0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC,
	}
	encoded := base64.StdEncoding.EncodeToString(original)

	if r := b.HandleRequest(InvokeRequest{
		ID: "2", Method: "goleo:fsWriteBinaryFile",
		Args: args(map[string]string{"path": target, "data": encoded}),
	}); r.Error != "" {
		t.Fatalf("writeBinaryFile: %s", r.Error)
	}

	// Byte-for-byte on disk, not merely the same length.
	onDisk, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, original) {
		t.Fatalf("on-disk bytes differ:\n got %v\nwant %v", onDisk, original)
	}

	// Reading back must return the same base64, not a U+FFFD-mangled string.
	r := b.HandleRequest(InvokeRequest{
		ID: "3", Method: "goleo:fsReadBinaryFile",
		Args: args(map[string]string{"path": target}),
	})
	if r.Error != "" {
		t.Fatalf("readBinaryFile: %s", r.Error)
	}
	got, ok := r.Result.(map[string]string)["data"]
	if !ok {
		t.Fatalf("readBinaryFile returned %#v", r.Result)
	}
	if got != encoded {
		t.Errorf("read-back base64 = %q, want %q", got, encoded)
	}
	decoded, derr := base64.StdEncoding.DecodeString(got)
	if derr != nil {
		t.Fatalf("read-back value is not valid base64: %v", derr)
	}
	if !bytes.Equal(decoded, original) {
		t.Errorf("decoded bytes differ:\n got %v\nwant %v", decoded, original)
	}
}
