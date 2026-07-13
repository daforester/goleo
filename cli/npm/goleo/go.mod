module github.com/daforester/goleo

go 1.26

require (
	github.com/crgimenes/glaze v0.0.31
	github.com/dop251/goja v0.0.0-20260701091749-b07b74453ea9
	github.com/ebitengine/purego v0.10.1
	github.com/gogpu/systray v0.1.1
	github.com/gorilla/websocket v1.5.1
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6
	golang.org/x/sys v0.46.0
)

require (
	github.com/dlclark/regexp2/v2 v2.2.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/go-webgpu/goffi v0.5.5 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/text v0.13.0 // indirect
)

replace github.com/crgimenes/glaze => github.com/daforester/glaze v0.0.32-goleo.2
