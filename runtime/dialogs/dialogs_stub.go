//go:build !windows && !darwin && !linux

package dialogs

import "fmt"

func platformOpenFile(opts FileDialogOptions) ([]string, error) {
	return nil, fmt.Errorf("dialogs not supported on this platform")
}

func platformSaveFile(opts FileDialogOptions) (string, error) {
	return "", fmt.Errorf("dialogs not supported on this platform")
}

func platformSelectFolder(opts FileDialogOptions) (string, error) {
	return "", fmt.Errorf("dialogs not supported on this platform")
}

func platformShowMessage(opts MessageBoxOptions) (string, error) {
	return "", fmt.Errorf("dialogs not supported on this platform")
}

func platformShowPrompt(opts PromptOptions) (string, error) {
	return "", fmt.Errorf("dialogs not supported on this platform")
}
