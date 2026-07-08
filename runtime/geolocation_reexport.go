//go:build !(android || ios) || goleo_geolocation

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/geolocation"
)

func RegisterGeolocation(b *Bridge) {
	b.Handle("goleo:geolocationGetCurrentPosition", func(ctx context.Context, args json.RawMessage) (any, error) {
		var opts geolocation.PositionOptions
		if len(args) > 0 {
			if err := json.Unmarshal(args, &opts); err != nil {
				return nil, err
			}
		}
		return geolocation.GetCurrentPosition(opts)
	})
}

// GeolocationProvider is re-exported so shells (e.g. the gomobile bridge) can
// inject a native backend without importing the sub-package directly.
type GeolocationProvider = geolocation.Provider

func SetGeolocationProvider(p GeolocationProvider) {
	geolocation.SetProvider(p)
}
