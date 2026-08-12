//go:build android || ios

package dialogs

import "errors"

// On mobile, native dialogs are only reachable from the native shell, which
// must register a Provider via SetProvider at startup — the generated
// backend/gomobile/dialogs.go exposes SetDialogsProvider for exactly that, and
// both scaffolded shells call it.
//
// Reaching one of these functions therefore means the shell did NOT register a
// provider, and it is a hard error, not a soft one. An earlier comment here
// claimed "the JS bridge falls back to web equivalents"; it does not. The
// fallbacks in bridge/src/dialogs.ts are gated on backendPresent(), and on
// mobile the Go backend is by definition present — so this error propagates to
// the caller instead. That is deliberate: a dialog that silently became a
// window.confirm the user never saw would let a destructive action proceed.

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
