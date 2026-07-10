// Package store is a simple persistent key/value store backed by a JSON file in
// the app data directory. It is pure Go and works identically on every target
// (desktop, mobile, server) — no native provider, no permission, no build tag.
// Values are stored as raw JSON.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
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
)

// Default returns the process-wide store at <appDataDir>/store.json.
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
		defStore, defErr = Open(filepath.Join(base, defaultAppName, storeFile))
	})
	return defStore, defErr
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

// persist writes the store atomically (temp file + rename).
func (s *Store) persist() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
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
