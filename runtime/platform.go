package runtime

import (
	"fmt"
	neturl "net/url"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type OSInfo struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type PlatformInfo struct {
	Platform  string `json:"platform"`
	IsMobile  bool   `json:"isMobile"`
	IsDesktop bool   `json:"isDesktop"`
	IsBrowser bool   `json:"isBrowser"`
}

func GetOSInfo() OSInfo {
	info := OSInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	switch runtime.GOOS {
	case "windows":
		info.Name = "Windows"
	case "darwin":
		info.Name = "macOS"
	case "linux":
		info.Name = "Linux"
	case "android":
		info.Name = "Android"
	case "ios":
		info.Name = "iOS"
	default:
		info.Name = runtime.GOOS
	}

	return info
}

func GetPlatformInfo() PlatformInfo {
	isMobile := runtime.GOOS == "android" || runtime.GOOS == "ios"
	isDesktop := runtime.GOOS == "windows" || runtime.GOOS == "darwin" || runtime.GOOS == "linux"

	return PlatformInfo{
		Platform:  runtime.GOOS,
		IsMobile:  isMobile,
		IsDesktop: isDesktop,
		IsBrowser: false,
	}
}

func GetArchInfo() string {
	return runtime.GOARCH
}

func GetEnvInfo(key string) string {
	// Only allow whitelisted env vars for security
	whitelist := map[string]bool{
		"HOME":         true,
		"USER":         true,
		"USERNAME":     true,
		"COMPUTERNAME": true,
		"PATH":         true,
		"SHELL":        true,
	}

	if !whitelist[key] {
		return ""
	}

	return strings.TrimSpace(Getenv(key))
}

// openURLExtraSchemes holds schemes the host app has explicitly opted into —
// currently the app's own Config.URLScheme, registered by App.Run. Guarded
// because Run and a bridge invoke can touch it from different goroutines.
var (
	openURLMu           sync.RWMutex
	openURLExtraSchemes = map[string]bool{}
)

// AllowURLScheme permits scheme (without "://") in OpenURL, on top of the
// safe-by-default set. App.Run calls this for Config.URLScheme so an app can
// always open its own deep links.
func AllowURLScheme(scheme string) {
	if scheme == "" {
		return
	}
	openURLMu.Lock()
	defer openURLMu.Unlock()
	openURLExtraSchemes[strings.ToLower(strings.TrimSuffix(scheme, "://"))] = true
}

// openURLAllowedSchemes is the default-safe set. goleo:openURL is a default
// builtin reachable by any script in the webview, and the OS handlers below
// will happily act on far more than web links: file:// and UNC paths expose the
// filesystem, and a path to an .exe/.app or a hostile registered scheme becomes
// arbitrary execution. Tauri and Electron restrict this the same way.
var openURLAllowedSchemes = map[string]bool{
	"http":   true,
	"https":  true,
	"mailto": true,
	"tel":    true,
}

// sortedAllowedSchemes lists every permitted scheme, for error messages.
func sortedAllowedSchemes() []string {
	openURLMu.RLock()
	defer openURLMu.RUnlock()
	out := make([]string, 0, len(openURLAllowedSchemes)+len(openURLExtraSchemes))
	for s := range openURLAllowedSchemes {
		out = append(out, s)
	}
	for s := range openURLExtraSchemes {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func allowedURLScheme(scheme string) bool {
	scheme = strings.ToLower(scheme)
	if openURLAllowedSchemes[scheme] {
		return true
	}
	openURLMu.RLock()
	defer openURLMu.RUnlock()
	return openURLExtraSchemes[scheme]
}

// checkOutboundURL is the guard for every path that hands a frontend-supplied URL
// to an OS default handler.
//
// It exists as its own function because there is more than one such path and the
// second one was missed: goleo:openURL was hardened while goleo:share — which ends
// up at the same rundll32/open/xdg-open call — kept passing whatever the frontend
// sent. `{"url":"file:///C:/Windows/System32/calc.exe"}` was arbitrary execution
// from any script in the webview. Anything new that opens a URL must call this.
func checkOutboundURL(op, rawURL string) error {
	url := strings.TrimSpace(rawURL)
	// Reject UNC paths (\\server\share) up front: they carry no scheme, so the
	// parse below would treat them as opaque and they are a live SMB-credential
	// leak on Windows.
	if strings.HasPrefix(url, `\\`) || strings.HasPrefix(url, "//") {
		return fmt.Errorf("%s: refusing to open UNC path %q", op, rawURL)
	}
	parsed, err := neturl.Parse(url)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	// A bare path ("C:\x\y.exe", "/tmp/x") parses with an empty scheme. Never
	// hand those to the OS handler.
	if parsed.Scheme == "" {
		return fmt.Errorf("%s: refusing to open %q (no URL scheme; only %s are allowed)",
			op, rawURL, strings.Join(sortedAllowedSchemes(), ", "))
	}
	if !allowedURLScheme(parsed.Scheme) {
		return fmt.Errorf("%s: scheme %q is not allowed (permitted: %s; add your own with Config.URLScheme)",
			op, parsed.Scheme, strings.Join(sortedAllowedSchemes(), ", "))
	}
	return nil
}

func OpenURL(rawURL string) error {
	url := strings.TrimSpace(rawURL)
	if err := checkOutboundURL("openURL", url); err != nil {
		return err
	}

	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}
