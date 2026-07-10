//go:build darwin && !ios

package share

import (
	"errors"
	"fmt"
	"os/exec"
)

func platformShare(data *ShareData) error {
	if data.URL == "" {
		return fmt.Errorf("share: text-only sharing %w on darwin", errors.ErrUnsupported)
	}
	return exec.Command("open", data.URL).Start()
}
