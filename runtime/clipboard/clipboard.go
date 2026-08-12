//go:build !(android || ios) || goleo_clipboard

package clipboard

import "sync"

// Provider is a native clipboard backend. On mobile the shell registers one
// via SetProvider (ClipboardManager on Android, UIPasteboard on iOS); on
// desktop the built-in platform implementation is used when no provider is
// set.
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

// ReadText returns the clipboard's text content, or "" if the clipboard holds
// no text. The text is returned verbatim — no trimming — so a round-trip
// through WriteText preserves leading/trailing whitespace.
func ReadText() (string, error) {
	if p := getProvider(); p != nil {
		return p.ReadText()
	}
	return platformReadText()
}

// WriteText replaces the clipboard's content with text. Every byte of text is
// treated as data, never as syntax for an underlying shell command.
func WriteText(text string) error {
	if p := getProvider(); p != nil {
		return p.WriteText(text)
	}
	return platformWriteText(text)
}
