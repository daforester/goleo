//go:build darwin && !ios

package clipboard

import "strings"

// pbpaste/pbcopy take the payload on stdin/stdout, so text is never exposed to
// shell parsing and needs no trimming on the way back.

func platformReadText() (string, error) {
	out, err := output("pbpaste")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func platformWriteText(text string) error {
	return runWithStdin(strings.NewReader(text), "pbcopy")
}
