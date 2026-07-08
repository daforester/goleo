//go:build !(android || ios) || goleo_camera

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/camera"
)

func RegisterCamera(b *Bridge) {
	b.Handle("goleo:cameraCapturePhoto", func(ctx context.Context, args json.RawMessage) (any, error) {
		return camera.CapturePhoto()
	})
	b.Handle("goleo:cameraStartStream", func(ctx context.Context, args json.RawMessage) (any, error) {
		var opts map[string]any
		if len(args) > 0 {
			if err := json.Unmarshal(args, &opts); err != nil {
				return nil, err
			}
		}
		return nil, camera.StartStream(opts)
	})
	b.Handle("goleo:cameraStopStream", func(ctx context.Context, args json.RawMessage) (any, error) {
		return nil, camera.StopStream()
	})
}

// CameraProvider is re-exported so shells (e.g. the gomobile bridge) can
// inject a native backend without importing the sub-package directly.
type CameraProvider = camera.Provider

func SetCameraProvider(p CameraProvider) {
	camera.SetProvider(p)
}
