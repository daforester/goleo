//go:build !(android || ios) || goleo_fs

package runtime

import (
	"context"
	"encoding/json"

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
		return map[string]string{"data": string(data)}, nil
	})

	b.Handle("goleo:fsWriteBinaryFile", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Path string `json:"path"`
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
		return fs.AppDataDir(req.AppName)
	})

	b.Handle("goleo:fsHomeDir", func(ctx context.Context, args json.RawMessage) (any, error) {
		return fs.HomeDir()
	})
}
