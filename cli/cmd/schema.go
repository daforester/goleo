package cmd

// CommandDecl describes a single built-in bridge command with its
// TypeScript types and optional feature association.
type CommandDecl struct {
	Method      string
	Args        string // TypeScript type for args, empty if none
	ReturnType  string
	Description string
	Feature     string // name in featureRegistry, empty if core
}

// KnownCommands is the authoritative list of all built-in bridge commands
// and their TypeScript types. Used by both the type generator and the scan
// command to detect feature usage.
var KnownCommands = []CommandDecl{
	// Core (always available, no build tag required)
	{Method: "goleo:getOS", ReturnType: "{ os: string; arch: string; name: string; version?: string }", Description: "Get OS information"},
	{Method: "goleo:getPlatform", ReturnType: "{ platform: string; isMobile: boolean; isDesktop: boolean; isBrowser: boolean }", Description: "Get platform info"},
	{Method: "goleo:getArch", ReturnType: "string", Description: "Get CPU architecture"},
	{Method: "goleo:getEnv", Args: "{ key: string }", ReturnType: "string", Description: "Get environment variable"},
	{Method: "goleo:openURL", Args: "{ url: string }", ReturnType: "void", Description: "Open URL in browser"},
	{Method: "goleo:notify", Args: "{ title: string; body?: string }", ReturnType: "void", Description: "Send notification"},
	{Method: "goleo:notificationPermissionGranted", ReturnType: "boolean", Description: "Check notification permission"},
	{Method: "goleo:requestNotificationPermission", ReturnType: "'granted' | 'denied' | 'default'", Description: "Request notification permission"},
	{Method: "goleo:showMessage", Args: "{ title: string; message: string }", ReturnType: "void", Description: "Log a message"},

	// Clipboard (feature: Clipboard / goleo_clipboard)
	{Method: "goleo:clipboardReadText", ReturnType: "{ text: string }", Description: "Read text from clipboard", Feature: "Clipboard"},
	{Method: "goleo:clipboardWriteText", Args: "{ text: string }", ReturnType: "void", Description: "Write text to clipboard", Feature: "Clipboard"},

	// Dialogs (feature: Dialogs / goleo_dialog)
	{Method: "goleo:dialogOpenFile", Args: "{ title?: string; defaultPath?: string; filters?: { name: string; patterns: string[] }[]; multiple?: boolean }", ReturnType: "string[]", Description: "Open file dialog", Feature: "Dialogs"},
	{Method: "goleo:dialogSaveFile", Args: "{ title?: string; defaultPath?: string; filters?: { name: string; patterns: string[] }[] }", ReturnType: "string", Description: "Save file dialog", Feature: "Dialogs"},
	{Method: "goleo:dialogSelectFolder", Args: "{ title?: string; defaultPath?: string }", ReturnType: "string", Description: "Select folder dialog", Feature: "Dialogs"},
	{Method: "goleo:dialogShowMessage", Args: "{ title?: string; message: string; icon?: 'info' | 'warning' | 'error' | 'question'; buttons?: string[] }", ReturnType: "{ button: string }", Description: "Show message box", Feature: "Dialogs"},
	{Method: "goleo:dialogShowPrompt", Args: "{ title?: string; message: string; defaultValue?: string }", ReturnType: "string", Description: "Show input prompt", Feature: "Dialogs"},

	// File System (feature: FileSystem / goleo_fs)
	{Method: "goleo:fsReadTextFile", Args: "{ path: string }", ReturnType: "string", Description: "Read text file", Feature: "FileSystem"},
	{Method: "goleo:fsWriteTextFile", Args: "{ path: string; content: string }", ReturnType: "void", Description: "Write text file", Feature: "FileSystem"},
	{Method: "goleo:fsReadBinaryFile", Args: "{ path: string }", ReturnType: "{ data: string }", Description: "Read binary file", Feature: "FileSystem"},
	{Method: "goleo:fsWriteBinaryFile", Args: "{ path: string; data: string }", ReturnType: "void", Description: "Write binary file", Feature: "FileSystem"},
	{Method: "goleo:fsListDir", Args: "{ path: string }", ReturnType: "{ name: string; path: string; isDir: boolean; size: number; modTime: string }[]", Description: "List directory contents", Feature: "FileSystem"},
	{Method: "goleo:fsDelete", Args: "{ path: string }", ReturnType: "void", Description: "Delete file or directory", Feature: "FileSystem"},
	{Method: "goleo:fsAppDataDir", Args: "{ appName?: string }", ReturnType: "string", Description: "Get app data directory", Feature: "FileSystem"},
	{Method: "goleo:fsHomeDir", ReturnType: "string", Description: "Get user home directory", Feature: "FileSystem"},

	// Geolocation (feature: Geolocation / goleo_geolocation)
	{Method: "goleo:geolocationGetCurrentPosition", Args: "{ enableHighAccuracy?: boolean; timeout?: number; maximumAge?: number }", ReturnType: "{ latitude: number; longitude: number; accuracy?: number }", Description: "Get current geographic position", Feature: "Geolocation"},

	// Battery (feature: Battery / goleo_battery)
	{Method: "goleo:batteryGetInfo", ReturnType: "{ level: number; charging: boolean; chargingTime?: number; dischargingTime?: number }", Description: "Get battery status information", Feature: "Battery"},

	// Share (feature: Share / goleo_share)
	{Method: "goleo:share", Args: "{ title?: string; text?: string; url?: string }", ReturnType: "void", Description: "Open the native share sheet", Feature: "Share"},

	// Store (persistent key/value — universal, no build tag / permission)
	{Method: "goleo:storeGet", Args: "{ key: string }", ReturnType: "{ value: unknown; found: boolean }", Description: "Read a value from the key/value store"},
	{Method: "goleo:storeSet", Args: "{ key: string; value: unknown }", ReturnType: "void", Description: "Write a value to the key/value store"},
	{Method: "goleo:storeDelete", Args: "{ key: string }", ReturnType: "void", Description: "Delete a key from the store"},
	{Method: "goleo:storeKeys", ReturnType: "string[]", Description: "List all keys in the store"},
	{Method: "goleo:storeClear", ReturnType: "void", Description: "Clear all keys from the store"},

	// Lifecycle
	{Method: "goleo:quit", ReturnType: "void", Description: "Request a graceful app shutdown (desktop)"},

	// Autostart (launch on login, desktop)
	{Method: "goleo:autostartEnable", ReturnType: "void", Description: "Register the app to launch on login"},
	{Method: "goleo:autostartDisable", ReturnType: "void", Description: "Remove the launch-on-login entry"},
	{Method: "goleo:autostartIsEnabled", ReturnType: "{ enabled: boolean }", Description: "Check if launch-on-login is registered"},

	// Updater (desktop auto-update)
	{Method: "goleo:updaterCheck", ReturnType: "{ available: boolean; version?: string; notes?: string }", Description: "Check for an available update"},
	{Method: "goleo:updaterApply", ReturnType: "void", Description: "Download and apply the latest update, then relaunch"},

	// WakeLock (feature: WakeLock / goleo_wakelock)
	{Method: "goleo:wakeLockRequest", Args: "{ type?: string }", ReturnType: "void", Description: "Request a wake lock to keep the screen on", Feature: "WakeLock"},
	{Method: "goleo:wakeLockRelease", ReturnType: "void", Description: "Release an active wake lock", Feature: "WakeLock"},

	// Vibration (feature: Vibration / goleo_vibration)
	{Method: "goleo:vibrate", Args: "{ pattern: number[] }", ReturnType: "void", Description: "Trigger vibration with a pattern", Feature: "Vibration"},

	// Sensors (feature: Sensors / goleo_sensors)
	{Method: "goleo:sensorStart", Args: "{ type: string }", ReturnType: "void", Description: "Start a sensor (accelerometer, gyroscope, etc.)", Feature: "Sensors"},
	{Method: "goleo:sensorStop", Args: "{ type: string }", ReturnType: "void", Description: "Stop a sensor", Feature: "Sensors"},

	// Camera (feature: Camera / goleo_camera)
	{Method: "goleo:cameraCapturePhoto", Args: "{ width?: number; height?: number }", ReturnType: "{ data: string; format: string }", Description: "Capture a photo from the camera", Feature: "Camera"},
	{Method: "goleo:cameraStartStream", Args: "{ width?: number; height?: number }", ReturnType: "void", Description: "Start a camera video stream", Feature: "Camera"},
	{Method: "goleo:cameraStopStream", ReturnType: "void", Description: "Stop an active camera stream", Feature: "Camera"},

	// Bluetooth / BLE (feature: Bluetooth / goleo_ble)
	{Method: "goleo:bleRequestDevice", Args: "{ filters?: Record<string, unknown> }", ReturnType: "{ id: string; name: string; rssi?: number }", Description: "Request a BLE device", Feature: "Bluetooth"},
	{Method: "goleo:bleConnect", Args: "{ deviceId: string }", ReturnType: "void", Description: "Connect to a BLE device", Feature: "Bluetooth"},
	{Method: "goleo:bleDisconnect", Args: "{ deviceId: string }", ReturnType: "void", Description: "Disconnect from a BLE device", Feature: "Bluetooth"},
	{Method: "goleo:bleRead", Args: "{ deviceId: string; service: string; characteristic: string }", ReturnType: "{ data: string }", Description: "Read data from a BLE characteristic", Feature: "Bluetooth"},
	{Method: "goleo:bleWrite", Args: "{ deviceId: string; service: string; characteristic: string; data: string }", ReturnType: "void", Description: "Write data to a BLE characteristic", Feature: "Bluetooth"},

	// NFC (feature: NFC / goleo_nfc)
	{Method: "goleo:nfcStartScan", ReturnType: "void", Description: "Start scanning for NFC tags", Feature: "NFC"},
	{Method: "goleo:nfcStopScan", ReturnType: "void", Description: "Stop scanning for NFC tags", Feature: "NFC"},
	{Method: "goleo:nfcWrite", Args: "{ records: { type: string; mediaType: string; data: string }[] }", ReturnType: "void", Description: "Write data to an NFC tag", Feature: "NFC"},

	// Background Sync (feature: Background / goleo_background)
	{Method: "goleo:backgroundRegisterSync", Args: "{ tag: string }", ReturnType: "void", Description: "Register a background sync event", Feature: "Background"},
	{Method: "goleo:backgroundPermissionGranted", ReturnType: "boolean", Description: "Check if background sync permission is granted", Feature: "Background"},
	{Method: "goleo:backgroundRequestPermission", ReturnType: "void", Description: "Request background sync permission", Feature: "Background"},

	// Push Notifications (feature: Push / goleo_push)
	{Method: "goleo:pushSubscribe", Args: "{ serverKey?: string }", ReturnType: "{ endpoint: string; keys: Record<string, string> }", Description: "Subscribe to push notifications", Feature: "Push"},
	{Method: "goleo:pushUnsubscribe", ReturnType: "void", Description: "Unsubscribe from push notifications", Feature: "Push"},
	{Method: "goleo:pushGetSubscription", ReturnType: "{ endpoint: string; keys: Record<string, string> } | null", Description: "Get the current push subscription", Feature: "Push"},
}
