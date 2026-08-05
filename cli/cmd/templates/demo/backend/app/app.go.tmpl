package app

import (
	"context"
	"log"

	"goleo/__GOLEO_APP_NAME__/backend/commands"
	"github.com/daforester/goleo/runtime"
)

// Options describes how this run differs across targets (desktop, mobile
// release, mobile dev). Everything else — command registration, feature
// wiring, shutdown logic — lives once, below, and applies to all of them.
type Options struct {
	DevMode    bool
	WindowMode runtime.WindowMode
	// EmbedFS is the caller's embedded frontend/dist (an embed.FS). Each
	// entry point embeds its own copy, since Go's //go:embed can only reach
	// files at or below the directory of the source file that declares it.
	EmbedFS any
}

// New builds this app's runtime.App. backend/main.go (desktop) and
// backend/gomobile/gomobile.go (mobile) both call this — add your own
// command registration and startup logic here, once, instead of
// per-entry-point.
func New(opts Options) *runtime.App {
	title := "__GOLEO_APP_NAME__"
	if opts.DevMode {
		title += " (dev)"
	}

	var a *runtime.App
	a = runtime.New(runtime.Config{
		Title:      title,
		Width:      1024,
		Height:     768,
		DevMode:    opts.DevMode,
		Port:       9842,
		WindowMode: opts.WindowMode,
		EmbedFS:    opts.EmbedFS,
		// InitJS: "init.js", // custom startup script path (default: init.js, then backend/init.js); desktop only
		OnStartup: func(ctx context.Context) {
			log.Println(title, "starting up...")
			runtime.RegisterBuiltins(a.Bridge())
			commands.Register(a.Bridge())
			commands.StartHeartbeat(a.Bridge())

			if opts.WindowMode == runtime.WindowModeWebview {
				// Desktop: clipboard, dialogs and filesystem.
				runtime.RegisterDesktopFeatures(a.Bridge())
				// Extra features with real desktop implementations, used by the
				// bundled demo pages. Remove any you don't need.
				runtime.RegisterBattery(a.Bridge())
				runtime.RegisterWakeLock(a.Bridge())
				runtime.RegisterGeolocation(a.Bridge())
				// Camera: native V4L2 on Linux; macOS/Windows fall back to the
				// webview's getUserMedia.
				runtime.RegisterCamera(a.Bridge())
				// NFC: native libnfc on Linux when built with -tags goleo_libnfc
				// (requires libnfc-dev + a reader). Otherwise it reports that no
				// desktop NFC is available.
				runtime.RegisterNFC(a.Bridge())
			}

			// Mobile permission-gated features. The build tag each one needs
			// (e.g. goleo_camera) is detected automatically from this file by
			// `goleo build android/ios` and `goleo emulate` — see
			// detectFeatureUsage in cli/cmd/scan.go — so you don't need to
			// pass -tags by hand. Just comment a line out if you don't use
			// that feature (it also drops the matching manifest permission).
			//
			// These have working native providers on Android (+ best-effort,
			// unverified iOS — no Xcode available to test it — see
			// AppDelegate.swift) and/or a WebView browser-API fallback that
			// Just Works, so the bundled demo pages exercise them by default:
			runtime.RegisterGeolocation(a.Bridge())
			runtime.RegisterCamera(a.Bridge())
			runtime.RegisterClipboard(a.Bridge())
			runtime.RegisterShare(a.Bridge())
			runtime.RegisterDialogs(a.Bridge())
			runtime.RegisterVibration(a.Bridge())
			runtime.RegisterWakeLock(a.Bridge())
			runtime.RegisterBattery(a.Bridge())
			runtime.RegisterFS(a.Bridge())
			runtime.RegisterSensors(a.Bridge())
			runtime.RegisterBackground(a.Bridge())
			// NFC: foreground-dispatch scan/write, native provider on
			// Android (best-effort, unverified iOS — CoreNFC needs a paid
			// Apple Developer entitlement + physical device either way).
			runtime.RegisterNFC(a.Bridge())
			// BLE: scan/connect/read/write via BluetoothLeScanner + GATT,
			// native provider on Android (best-effort, unverified iOS).
			runtime.RegisterBLE(a.Bridge())
			//
			// Commented out because it needs your own Firebase/Apple
			// Developer project credentials, which can't be provided
			// generically no matter what native code exists:
			// runtime.RegisterPush(a.Bridge())
		},
		OnShutdown: func(ctx context.Context) {
			log.Println(title, "shutting down...")
		},
	})
	return a
}
