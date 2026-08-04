//go:build darwin && !ios

package share

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
)

// platformShare hands a URL to the OS default handler — the closest desktop
// equivalent to a share sheet available without native UI. Text-only shares have no
// shell path; the TS layer then falls back to the Web Share API / clipboard.
//
// The argv comes from osShareCommand (share.go) so all three desktops are built and
// tested in one place; see the comment there for why no shell may be involved.
func platformShare(data *ShareData) error {
	if data.URL == "" {
		return fmt.Errorf("share: text-only sharing %w on darwin", errors.ErrUnsupported)
	}
	name, args := osShareCommand(runtime.GOOS, data.URL)
	return exec.Command(name, args...).Start()
}
