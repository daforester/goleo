package runtime

// TrayItem is one system-tray menu entry.
type TrayItem struct {
	Label   string
	OnClick func()
}

// TrayConfig configures an optional system tray icon + menu. Set Config.Tray
// (with Config.Background) to run as a tray app. Icon is PNG bytes.
type TrayConfig struct {
	Icon    []byte
	Tooltip string
	Items   []TrayItem
}
