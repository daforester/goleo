//go:build android || ios

package dialogs

import "errors"

// On mobile, native dialogs are only reachable from the native shell, which
// must register a Provider via SetProvider at startup. Without one, calls
// fail and the JS bridge falls back to web equivalents (<input type="file">,
// window.alert, window.prompt).

var errNoProvider = errors.New("dialogs: no native provider registered: the mobile shell must call SetProvider at startup")

func platformOpenFile(opts FileDialogOptions) ([]string, error) {
	return nil, errNoProvider
}

func platformSaveFile(opts FileDialogOptions) (string, error) {
	return "", errNoProvider
}

func platformSelectFolder(opts FileDialogOptions) (string, error) {
	return "", errNoProvider
}

func platformShowMessage(opts MessageBoxOptions) (string, error) {
	return "", errNoProvider
}

func platformShowPrompt(opts PromptOptions) (string, error) {
	return "", errNoProvider
}
