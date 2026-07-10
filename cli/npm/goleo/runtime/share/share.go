//go:build !(android || ios) || goleo_share

package share

import "sync"

// ShareData is the payload for the native share sheet. All fields are optional;
// at least one of Text or URL is normally set. Mirrors the Web Share API.
type ShareData struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	URL   string `json:"url"`
}

// Provider is a native share backend. On mobile the shell registers one via
// SetProvider (Android Intent.ACTION_SEND / iOS UIActivityViewController); on
// desktop the built-in platform implementation is used when no provider is set.
type Provider interface {
	Share(data *ShareData) error
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

func Share(data *ShareData) error {
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p != nil {
		return p.Share(data)
	}
	return platformShare(data)
}
