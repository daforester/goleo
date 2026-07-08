package runtime

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type OSInfo struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type PlatformInfo struct {
	Platform    string `json:"platform"`
	IsMobile    bool   `json:"isMobile"`
	IsDesktop   bool   `json:"isDesktop"`
	IsBrowser   bool   `json:"isBrowser"`
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
		"HOME":       true,
		"USER":       true,
		"USERNAME":   true,
		"COMPUTERNAME": true,
		"PATH":       true,
		"SHELL":      true,
	}

	if !whitelist[key] {
		return ""
	}

	return strings.TrimSpace(Getenv(key))
}

func OpenURL(url string) error {
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
