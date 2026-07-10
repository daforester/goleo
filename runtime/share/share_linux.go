//go:build linux && !android

package share

import (
	"errors"
	"fmt"
	"os/exec"
)

func platformShare(data *ShareData) error {
	if data.URL == "" {
		return fmt.Errorf("share: text-only sharing %w on linux", errors.ErrUnsupported)
	}
	return exec.Command("xdg-open", data.URL).Start()
}
