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
	// Hand the URL to rundll32 as a plain argv element rather than through
	// `cmd /c start`. cmd re-parses its own command line and treats `&` as a
	// command separator, and Go's syscall.EscapeArg only quotes arguments
	// containing space/tab/newline/quote — not `&` — so a frontend-supplied URL
	// like `http://x&calc` executed `calc`. rundll32 involves no shell.
	// (runtime.OpenURL uses the same mechanism; this package can't import
	// runtime without a cycle.)
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", data.URL).Start()
}
