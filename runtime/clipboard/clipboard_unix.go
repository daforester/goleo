//go:build (darwin && !ios) || (linux && !android)

package clipboard

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

func output(name string, args ...string) ([]byte, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return nil, cmdError(name, err)
	}
	return out, nil
}

func runWithStdin(stdin io.Reader, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = stdin
	// Stderr is deliberately left unset: xclip forks a background process to
	// serve the selection, and it keeps a captured stderr pipe open, so Wait
	// would block on it indefinitely. A missing binary still surfaces as an
	// *exec.Error, which reads clearly on its own.
	if err := cmd.Run(); err != nil {
		return cmdError(name, err)
	}
	return nil
}

func cmdError(name string, err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("clipboard: %s: %w: %s", name, err, strings.TrimSpace(string(exitErr.Stderr)))
	}
	return fmt.Errorf("clipboard: %s: %w", name, err)
}
