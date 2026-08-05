//go:build linux && !android

package clipboard

import "strings"

// xclip takes the payload on stdin/stdout, so text is never exposed to shell
// parsing and needs no trimming on the way back.

// xclip exits non-zero with this on its stderr when the clipboard is empty or
// its owner offers no text target — "no text", not a failure, so it is
// reported the same way Windows and macOS report it: an empty string.
const noTextTarget = "target STRING not available"

func platformReadText() (string, error) {
	out, err := output("xclip", "-o", "-selection", "clipboard")
	if err != nil {
		if strings.Contains(err.Error(), noTextTarget) {
			return "", nil
		}
		return "", err
	}
	return string(out), nil
}

func platformWriteText(text string) error {
	return runWithStdin(strings.NewReader(text), "xclip", "-selection", "clipboard")
}
