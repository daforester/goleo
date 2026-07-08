package runtime

import (
	"embed"
	"io/fs"
)

func NewEmbedFS(e embed.FS, subDir string) (fs.FS, error) {
	if subDir != "" {
		return fs.Sub(e, subDir)
	}
	return e, nil
}
