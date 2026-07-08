//go:build !(android || ios) || goleo_clipboard

package clipboard

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// Provider is a native clipboard backend. On mobile the shell registers one
// via SetProvider (ClipboardManager on Android, UIPasteboard on iOS); on
// desktop the built-in shell-command implementations are used when no
// provider is set.
type Provider interface {
	ReadText() (string, error)
	WriteText(text string) error
}

var (
	providerMu sync.RWMutex
	provider   Provider
)

func SetProvider(p Provider) {
	providerMu.Lock()
	defer providerMu.Unlock()
	provider = p
}

func getProvider() Provider {
	providerMu.RLock()
	defer providerMu.RUnlock()
	return provider
}

func ReadText() (string, error) {
	if p := getProvider(); p != nil {
		return p.ReadText()
	}
	switch runtime.GOOS {
	case "windows":
		return readWindows()
	case "darwin":
		return readDarwin()
	case "linux":
		return readLinux()
	default:
		return "", errors.New("clipboard not supported on this platform")
	}
}

func WriteText(text string) error {
	if p := getProvider(); p != nil {
		return p.WriteText(text)
	}
	switch runtime.GOOS {
	case "windows":
		return writeWindows(text)
	case "darwin":
		return writeDarwin(text)
	case "linux":
		return writeLinux(text)
	default:
		return errors.New("clipboard not supported on this platform")
	}
}

func readWindows() (string, error) {
	out, err := exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func writeWindows(text string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Set-Clipboard", text)
	return cmd.Run()
}

func readDarwin() (string, error) {
	out, err := exec.Command("pbpaste").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func writeDarwin(text string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func readLinux() (string, error) {
	out, err := exec.Command("xclip", "-o", "-selection", "clipboard").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func writeLinux(text string) error {
	cmd := exec.Command("xclip", "-selection", "clipboard")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
