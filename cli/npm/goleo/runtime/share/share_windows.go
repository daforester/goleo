//go:build windows

package share

import (
	"errors"
	"fmt"
	"os/exec"
)

// platformShare hands a URL to the OS default handler — the closest desktop
// equivalent to a share sheet available without native UI. Text-only shares
// have no shell path; the TS layer then falls back to the Web Share API /
// clipboard.
func platformShare(data *ShareData) error {
	if data.URL == "" {
		return fmt.Errorf("share: text-only sharing %w on windows", errors.ErrUnsupported)
	}
	return exec.Command("cmd", "/c", "start", "", data.URL).Start()
}
