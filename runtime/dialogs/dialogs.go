package dialogs

import "sync"

type FileFilter struct {
	Name     string   `json:"name"`
	Patterns []string `json:"patterns"`
}

type FileDialogOptions struct {
	Title       string       `json:"title"`
	DefaultPath string       `json:"defaultPath,omitempty"`
	Filters     []FileFilter `json:"filters,omitempty"`
	Multiple    bool         `json:"multiple,omitempty"`
}

type MessageBoxOptions struct {
	Title   string   `json:"title"`
	Message string   `json:"message"`
	Icon    string   `json:"icon,omitempty"`
	Buttons []string `json:"buttons,omitempty"`
}

type PromptOptions struct {
	Title        string `json:"title"`
	Message      string `json:"message"`
	DefaultValue string `json:"defaultValue"`
}

// Provider is a native dialogs backend. On mobile the shell (Android
// Activity / iOS AppDelegate) registers one via SetProvider; on desktop the
// built-in platform implementations are used when no provider is set.
type Provider interface {
	OpenFile(opts FileDialogOptions) ([]string, error)
	SaveFile(opts FileDialogOptions) (string, error)
	SelectFolder(opts FileDialogOptions) (string, error)
	ShowMessage(opts MessageBoxOptions) (string, error)
	ShowPrompt(opts PromptOptions) (string, error)
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

func OpenFile(opts FileDialogOptions) ([]string, error) {
	if opts.Title == "" {
		opts.Title = "Open File"
	}
	if p := getProvider(); p != nil {
		return p.OpenFile(opts)
	}
	return platformOpenFile(opts)
}

func SaveFile(opts FileDialogOptions) (string, error) {
	if opts.Title == "" {
		opts.Title = "Save File"
	}
	if p := getProvider(); p != nil {
		return p.SaveFile(opts)
	}
	return platformSaveFile(opts)
}

func SelectFolder(opts FileDialogOptions) (string, error) {
	if opts.Title == "" {
		opts.Title = "Select Folder"
	}
	if p := getProvider(); p != nil {
		return p.SelectFolder(opts)
	}
	return platformSelectFolder(opts)
}

func ShowMessage(opts MessageBoxOptions) (string, error) {
	if opts.Title == "" {
		opts.Title = "Message"
	}
	if p := getProvider(); p != nil {
		return p.ShowMessage(opts)
	}
	return platformShowMessage(opts)
}

func ShowPrompt(opts PromptOptions) (string, error) {
	if opts.Title == "" {
		opts.Title = "Input"
	}
	if p := getProvider(); p != nil {
		return p.ShowPrompt(opts)
	}
	return platformShowPrompt(opts)
}

// platform functions are implemented per-platform:
// - dialogs_windows.go (build tag: windows)
// - dialogs_darwin.go  (build tag: darwin && !ios)
// - dialogs_linux.go   (build tag: linux && !android)
// - dialogs_mobile.go  (build tag: android || ios) — requires SetProvider
// - dialogs_stub.go    (build tag: !windows && !darwin && !linux)
