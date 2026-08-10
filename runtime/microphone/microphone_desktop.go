//go:build !android && !ios

package microphone

import (
	"errors"
	"fmt"
)

// Desktop has no OS-level microphone permission to query: the WebView's getUserMedia
// prompt IS the permission model. ErrUnsupported (not a generic error) is what lets the JS
// wrapper tell "no native path here, use the browser API" apart from a real failure.
func platformPermissionGranted() (bool, error) {
	return false, fmt.Errorf("microphone: permission state %w on desktop", errors.ErrUnsupported)
}

func platformRequestPermission() (string, error) {
	return "", fmt.Errorf("microphone: permission request %w on desktop", errors.ErrUnsupported)
}
