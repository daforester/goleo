// Package store is a simple persistent key/value store backed by a JSON file in
// the app data directory. It is pure Go and works identically on every target
// (desktop, mobile, server) — no native provider, no permission, no build tag.
// Values are stored as raw JSON.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	defaultAppName = "goleo-app"
	storeFile      = "store.json"
)

// Store is a concurrency-safe JSON-file-backed key/value store.
type Store struct {
	mu   sync.RWMutex
	path string
	data map[string]json.RawMessage
}

var (
	defOnce  sync.Once
	defStore *Store
	defErr   error

	appNameMu sync.RWMutex
	appName   = defaultAppName
)

// SetAppName names the directory the default store lives in:
// <UserConfigDir>/<appName>/store.json. App.New calls this with Config.AppID
// (falling back to Title).
//
// Without it every goleo app on a machine shared ONE store.json under
// "goleo-app" — two different apps read and clobbered each other's keys. Call it
// before the first Default(), which App.New does; afterwards the path is fixed for
// the process.
func SetAppName(name string) {
	if name == "" {
		return
	}
	appNameMu.Lock()
	defer appNameMu.Unlock()
	appName = sanitizeAppName(name)
}

// sanitizeAppName keeps the name usable as a single directory element. Title is an
// arbitrary human string ("My App: 2.0"), so it can contain separators and
// characters Windows rejects in a path.
func sanitizeAppName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '-'
		}
		if r < 0x20 {
			return '-'
		}
		return r
	}, name)
	cleaned = strings.Trim(strings.TrimSpace(cleaned), ".")
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return defaultAppName
	}
	return cleaned
}

// Default returns the process-wide store at <appDataDir>/<appName>/store.json.
func Default() (*Store, error) {
	defOnce.Do(func() {
		// Self-contained (no runtime/fs import) so the package compiles on
		// mobile, where fs is build-tag-gated behind goleo_fs. Mirrors
		// fs.AppDataDir's logic.
		base, err := os.UserConfigDir()
		if err != nil {
			defErr = err
			return
		}
		appNameMu.RLock()
		name := appName
		appNameMu.RUnlock()

		path := filepath.Join(base, name, storeFile)
		// Adopt the pre-rename store if this app has none yet. Without this the
		// rename is silent DATA LOSS: an existing app upgrading to a version that
		// sets AppID would look at a fresh empty directory and every stored key
		// would appear to have vanished.
		if name != defaultAppName {
			migrateLegacyStore(filepath.Join(base, defaultAppName, storeFile), path)
		}
		defStore, defErr = Open(path)
	})
	return defStore, defErr
}

// migrateLegacyStore copies the shared "goleo-app" store to this app's own path,
// once, if the app has nothing there yet. It copies rather than moves: other apps
// on the machine may still be reading the legacy file, and a half-finished move
// would strand them.
func migrateLegacyStore(legacy, current string) {
	if legacy == current {
		return
	}
	if _, err := os.Stat(current); err == nil {
		return // this app already has its own store
	}
	data, err := os.ReadFile(legacy)
	if err != nil || len(data) == 0 {
		return // nothing to inherit
	}
	if err := os.MkdirAll(filepath.Dir(current), 0o755); err != nil {
		return
	}
	// Write through a temp file so an interrupted migration cannot leave a
	// truncated store behind.
	tmp := current + ".migrating"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, current); err != nil {
		os.Remove(tmp)
	}
}

// Open loads (or lazily creates) a store at the given file path.
func Open(path string) (*Store, error) {
	s := &Store{path: path, data: map[string]json.RawMessage{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // not created yet — empty store
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, &s.data)
}

// persist writes the store atomically: unique temp file, fsync, rename.
//
// Two details the previous version got wrong. The temp path was a fixed
// s.path+".tmp", so two writers — a second instance, or the single-instance
// forwarder racing the primary — used the SAME temp file and could interleave into
// a corrupt store. And it never fsynced, so a crash or power loss after the rename
// could leave a zero-length or partial file where a valid store used to be: the
// rename is atomic with respect to the directory entry, not to the data reaching
// the disk.
func (s *Store) persist() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(s.data)
	if err != nil {
		return err
	}

	f, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// Any failure from here on must not leave the temp file behind.
	defer func() {
		if tmp != "" {
			os.Remove(tmp)
		}
	}()

	if err := f.Chmod(0o600); err != nil && !os.IsPermission(err) {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	tmp = "" // renamed successfully; nothing to clean up
	return nil
}

func (s *Store) Get(key string) (json.RawMessage, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *Store) Set(key string, value json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return s.persist()
}

func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return s.persist()
}

func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = map[string]json.RawMessage{}
	return s.persist()
}
