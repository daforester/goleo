//go:build !(android || ios) || goleo_sensors

package runtime

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime/sensors"
)

func RegisterSensors(b *Bridge) {
	b.Handle("goleo:sensorStart", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		return nil, sensors.StartSensor(req.Type)
	})
	b.Handle("goleo:sensorStop", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		return nil, sensors.StopSensor(req.Type)
	})
}

// SensorsProvider is re-exported so shells (e.g. the gomobile bridge) can
// inject a native backend without importing the sub-package directly.
type SensorsProvider = sensors.Provider

func SetSensorsProvider(p SensorsProvider) {
	sensors.SetProvider(p)
}
