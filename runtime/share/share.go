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

// osShareCommand returns the argv for handing a URL to the OS default handler.
//
// Built here, without a build constraint and taking goos as a parameter, so all
// three desktops' argv can be asserted from any host. The property it protects is
// invisible in a working share: the URL must be its own argv element with NO shell
// in the pipeline. `cmd /c start <url>` used to be the Windows path, and cmd
// re-parses its own command line — Go's syscall.EscapeArg quotes only
// space/tab/newline/quote, not `&` — so a frontend URL of `http://x&calc` ran calc.
// rundll32, open and xdg-open all take the URL as a plain argument.
//
// Callers must still validate the URL itself: runtime's goleo:share handler applies
// the same scheme allow-list as goleo:openURL, because these handlers will act on
// file:// and on paths to executables.
func osShareCommand(goos, url string) (string, []string) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		return "open", []string{url}
	default:
		return "xdg-open", []string{url}
	}
}
