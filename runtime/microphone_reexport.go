//go:build !(android || ios) || goleo_microphone

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/microphone"
)

// RegisterMicrophone opts the app in to microphone access.
//
// Recording itself happens in the WebView (getUserMedia + MediaRecorder) and needs no Go
// code; these two commands exist because the WebView cannot check permission without
// starting a capture, and on mobile that check is a native API. Registering this is also
// what puts RECORD_AUDIO in the generated Android manifest, which is why it is separate
// from RegisterCamera — see the package comment.
func RegisterMicrophone(b *Bridge) {
	b.Handle("goleo:microphonePermission", func(ctx context.Context, args json.RawMessage) (any, error) {
		granted, err := microphone.PermissionGranted()
		if err != nil {
			return nil, err
		}
		return map[string]bool{"granted": granted}, nil
	})

	b.Handle("goleo:microphoneRequestPermission", func(ctx context.Context, args json.RawMessage) (any, error) {
		status, err := microphone.RequestPermission()
		if err != nil {
			return nil, err
		}
		return map[string]string{"status": status}, nil
	})
}

// MicrophoneProvider is re-exported so shells (e.g. the gomobile bridge) can inject a
// native backend without importing the sub-package directly.
type MicrophoneProvider = microphone.Provider

func SetMicrophoneProvider(p MicrophoneProvider) {
	microphone.SetProvider(p)
}
