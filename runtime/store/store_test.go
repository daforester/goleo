package store

import (
	"path/filepath"
	"testing"
)

func TestStoreRoundTripAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("missing"); ok {
		t.Fatal("expected missing key to report not-found")
	}

	if err := s.Set("k", []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if v, ok := s.Get("k"); !ok || string(v) != `{"a":1}` {
		t.Fatalf("Get(k) = %q, ok=%v", v, ok)
	}

	// Persistence: a fresh handle on the same file sees the written value.
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := s2.Get("k"); !ok || string(v) != `{"a":1}` {
		t.Fatalf("after reopen Get(k) = %q, ok=%v", v, ok)
	}
	if keys := s2.Keys(); len(keys) != 1 || keys[0] != "k" {
		t.Fatalf("Keys() = %v", keys)
	}

	if err := s2.Delete("k"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.Get("k"); ok {
		t.Fatal("expected key deleted")
	}

	_ = s2.Set("a", []byte(`1`))
	_ = s2.Set("b", []byte(`2`))
	if err := s2.Clear(); err != nil {
		t.Fatal(err)
	}
	if keys := s2.Keys(); len(keys) != 0 {
		t.Fatalf("expected empty after Clear, got %v", keys)
	}
}
