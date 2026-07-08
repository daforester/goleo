//go:build !(android || ios) || goleo_fs

package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

func ReadTextFile(path string) (string, error) {
	if err := validatePath(path); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

func WriteTextFile(path, content string) error {
	if err := validatePath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func ReadBinaryFile(path string) ([]byte, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return data, nil
}

func WriteBinaryFile(path string, data []byte) error {
	if err := validatePath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func ListDir(path string) ([]FileEntry, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("list dir: %w", err)
	}
	var out []FileEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, FileEntry{
			Name:    e.Name(),
			Path:    filepath.Join(path, e.Name()),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return out, nil
}

func Delete(path string) error {
	if err := validatePath(path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

func AppDataDir(appName string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	return filepath.Join(base, appName), nil
}

func HomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return home, nil
}

func validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path must not be empty")
	}
	clean := filepath.Clean(path)
	if strings.HasPrefix(clean, "..") || strings.Contains(clean, "..\\") {
		return fmt.Errorf("path traversal detected: %s", path)
	}
	return nil
}
