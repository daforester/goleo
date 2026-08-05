package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeAppName(t *testing.T) {
	cases := map[string]string{
		"goleo-demo":    "goleo-demo",
		"My App":        "My App",
		"My App: 2.0":   "My App- 2.0", // ':' is illegal in a Windows path
		"a/b":           "a-b",
		`a\b`:           "a-b",
		"trailing.":     "trailing",
		"  spaced  ":    "spaced",
		"a*b?c<d>e|f":   "a-b-c-d-e-f",
		"with\"quote":   "with-quote",
		"line\nbreak":   "line-break",
		"":              defaultAppName, // SetAppName ignores "", but be defensive
		".":             defaultAppName,
		"..":            defaultAppName,
		"...":           defaultAppName,
		"legit.name.v2": "legit.name.v2",
	}
	for in, want := range cases {
		if got := sanitizeAppName(in); got != want {
			t.Errorf("sanitizeAppName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The rename is silent data loss without this: an existing app that upgrades to a
// version setting AppID would look at a fresh empty directory, and every stored key
// would appear to have vanished.
func TestMigrateLegacyStoreAdoptsExistingData(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, defaultAppName, storeFile)
	current := filepath.Join(base, "my-app", storeFile)

	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{"token":"\"abc\"","count":"7"}`
	if err := os.WriteFile(legacy, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}

	migrateLegacyStore(legacy, current)

	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("migrated store missing: %v", err)
	}
	if string(got) != payload {
		t.Errorf("migrated contents = %s, want %s", got, payload)
	}
	// Copy, not move: other apps on the machine may still be reading the legacy
	// file, and a half-finished move would strand them.
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("legacy store should be left in place, got %v", err)
	}
}

// An app that already has its own store must never have it overwritten by the
// shared legacy one — that would be data loss in the other direction.
func TestMigrateLegacyStoreNeverOverwrites(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, defaultAppName, storeFile)
	current := filepath.Join(base, "my-app", storeFile)

	for path, body := range map[string]string{
		legacy:  `{"from":"\"legacy\""}`,
		current: `{"from":"\"mine\""}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	migrateLegacyStore(legacy, current)

	got, _ := os.ReadFile(current)
	if string(got) != `{"from":"\"mine\""}` {
		t.Errorf("existing store was overwritten: %s", got)
	}
}

func TestMigrateLegacyStoreHandlesNothingToDo(t *testing.T) {
	base := t.TempDir()
	current := filepath.Join(base, "my-app", storeFile)

	// No legacy file at all — must not create anything or panic.
	migrateLegacyStore(filepath.Join(base, defaultAppName, storeFile), current)
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Errorf("nothing should have been created, got %v", err)
	}

	// Same path both sides is a no-op, not a self-copy.
	migrateLegacyStore(current, current)
}

// persist used a fixed <path>.tmp, so two writers shared one temp file and could
// interleave into a corrupt store. Each write must use a distinct temp name and
// leave none behind.
func TestPersistLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, storeFile))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := s.Set("k", json.RawMessage(`"v"`)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != storeFile {
			t.Errorf("stray file left behind: %s", e.Name())
		}
	}
}

// Concurrent writers must not corrupt the file. With a shared fixed temp path this
// was a real race; with unique temps each rename is atomic.
func TestConcurrentWritesStayValid(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, storeFile))
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	for i := 0; i < 4; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 25; j++ {
				_ = s.Set("key", json.RawMessage(`"value"`))
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		<-done
	}

	raw, err := os.ReadFile(filepath.Join(dir, storeFile))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("store file is not valid JSON after concurrent writes: %v\n%s", err, raw)
	}
}
