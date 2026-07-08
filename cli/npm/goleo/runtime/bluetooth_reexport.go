//go:build !(android || ios) || goleo_ble

package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/daforester/goleo/runtime/bluetooth"
)

func RegisterBLE(b *Bridge) {
	b.Handle("goleo:bleRequestDevice", func(ctx context.Context, args json.RawMessage) (any, error) {
		var filters map[string]any
		if len(args) > 0 {
			if err := json.Unmarshal(args, &filters); err != nil {
				return nil, err
			}
		}
		return bluetooth.RequestDevice(filters)
	})
	b.Handle("goleo:bleConnect", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			DeviceID string `json:"deviceID"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		return nil, bluetooth.Connect(req.DeviceID)
	})
	b.Handle("goleo:bleDisconnect", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			DeviceID string `json:"deviceID"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		return nil, bluetooth.Disconnect(req.DeviceID)
	})
	b.Handle("goleo:bleRead", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			DeviceID       string `json:"deviceID"`
			Service        string `json:"service"`
			Characteristic string `json:"characteristic"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		data, err := bluetooth.Read(req.DeviceID, req.Service, req.Characteristic)
		if err != nil {
			return nil, err
		}
		return map[string]string{"data": base64.StdEncoding.EncodeToString(data)}, nil
	})
	b.Handle("goleo:bleWrite", func(ctx context.Context, args json.RawMessage) (any, error) {
		var req struct {
			DeviceID       string `json:"deviceID"`
			Service        string `json:"service"`
			Characteristic string `json:"characteristic"`
			Data           string `json:"data"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, err
		}
		data, err := base64.StdEncoding.DecodeString(req.Data)
		if err != nil {
			return nil, err
		}
		return nil, bluetooth.Write(req.DeviceID, req.Service, req.Characteristic, data)
	})
}

// BLEProvider is re-exported so shells (e.g. the gomobile bridge) can
// inject a native backend without importing the sub-package directly.
type BLEProvider = bluetooth.Provider

func SetBLEProvider(p BLEProvider) {
	bluetooth.SetProvider(p)
}
