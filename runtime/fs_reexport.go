//go:build !(android || ios) || goleo_fs

package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/daforester/goleo/runtime/fs"
)

func RegisterFS(b *Bridge) {
	b.Handle("goleo:fsReadTextFile", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		path, err := b.checkFSPath(req.Path, fsRead)
		if err != nil {
			return nil, err
		}
		return fs.ReadTextFile(path)
	})

	b.Handle("goleo:fsWriteTextFile", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		path, err := b.checkFSPath(req.Path, fsWrite)
		if err != nil {
			return nil, err
		}
		return nil, fs.WriteTextFile(path, req.Content)
	})

	b.Handle("goleo:fsReadBinaryFile", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		path, err := b.checkFSPath(req.Path, fsRead)
		if err != nil {
			return nil, err
		}
		data, err := fs.ReadBinaryFile(path)
		if err != nil {
			return nil, err
		}
		// base64, not string(data). Returning the raw bytes as a Go string put
		// arbitrary binary through encoding/json, which replaces every invalid UTF-8
		// sequence with U+FFFD — so a PNG was already corrupted on the wire, before
		// the frontend saw it. base64 is exactly what encoding/json does for []byte,
		// so the two directions now agree.
		return map[string]string{"data": base64.StdEncoding.EncodeToString(data)}, nil
	})

	b.Handle("goleo:fsWriteBinaryFile", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Path string `json:"path"`
			// []byte means encoding/json expects a base64 STRING here. That was
			// always the contract; the TS side was sending TextDecoder output
			// instead, so any non-ASCII payload either failed to unmarshal or
			// produced the wrong bytes. Both sides use base64 now.
			Data []byte `json:"data"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		path, err := b.checkFSPath(req.Path, fsWrite)
		if err != nil {
			return nil, err
		}
		return nil, fs.WriteBinaryFile(path, req.Data)
	})

	b.Handle("goleo:fsListDir", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		path, err := b.checkFSPath(req.Path, fsRead)
		if err != nil {
			return nil, err
		}
		return fs.ListDir(path)
	})

	b.Handle("goleo:fsDelete", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		path, err := b.checkFSPath(req.Path, fsWrite)
		if err != nil {
			return nil, err
		}
		return nil, fs.Delete(path)
	})

	b.Handle("goleo:fsAppDataDir", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			AppName string `json:"appName"`
		}
		if len(args) > 0 {
			if err := json.Unmarshal(args, &req); err != nil {
				return nil, err
			}
		}
		if req.AppName == "" {
			req.AppName = "goleo"
		}
		// appName becomes a path element, so it must not be able to climb out:
		// filepath.Join(base, "../../etc") cleans to /etc, which would turn this
		// into an arbitrary-directory grant.
		if !validAppDataName(req.AppName) {
			return nil, fmt.Errorf("fs: invalid appName %q (no path separators or %q)", req.AppName, "..")
		}
		dir, err := fs.AppDataDir(req.AppName)
		if err != nil {
			return nil, err
		}
		// Bring it into scope. The plugin is vending this path as "where your data
		// goes", so returning it and then refusing writes to it is incoherent — and
		// it broke the demo, which asks for appDataDir("goleo-demo") while the scope
		// root is named after the app's AppID/Title. The name is validated above, so
		// this cannot be used to widen scope arbitrarily.
		b.AddFSRoot(dir)
		return dir, nil
	})

	// Note: fsHomeDir deliberately does NOT grant anything. It answers "where is
	// home", which is informational; granting it would hand the whole user profile
	// back and defeat the confinement. Writing under it still requires an explicit
	// Policy.FSRoots entry or a path the user picked in a dialog.
	b.Handle("goleo:fsHomeDir", func(ctx context.Context, args json.RawMessage) (any, error) {
		return fs.HomeDir()
	})
}
