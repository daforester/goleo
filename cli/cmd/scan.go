package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Feature struct {
	Name          string
	BuildTag      string
	Permissions   []string          // Android permissions
	IOSUsageDescs map[string]string // iOS Info.plist keys → descriptions
}

var featureRegistry = []Feature{
	{
		Name:        "Clipboard",
		BuildTag:    "goleo_clipboard",
		Permissions: []string{},
	},
	{
		Name:        "Dialogs",
		BuildTag:    "goleo_dialog",
		Permissions: []string{"android.permission.READ_EXTERNAL_STORAGE"},
		IOSUsageDescs: map[string]string{
			"NSPhotoLibraryUsageDescription": "Access photo library for file picking",
		},
	},
	{
		Name:        "FileSystem",
		BuildTag:    "goleo_fs",
		Permissions: []string{"android.permission.READ_EXTERNAL_STORAGE", "android.permission.WRITE_EXTERNAL_STORAGE"},
		IOSUsageDescs: map[string]string{
			"NSDocumentsFolderUsageDescription": "Access documents for file management",
		},
	},
	{
		Name:        "Geolocation",
		BuildTag:    "goleo_geolocation",
		Permissions: []string{"android.permission.ACCESS_FINE_LOCATION"},
		IOSUsageDescs: map[string]string{
			"NSLocationWhenInUseUsageDescription": "Access location for GPS features",
		},
	},
	{
		Name:        "Battery",
		BuildTag:    "goleo_battery",
		Permissions: []string{"android.permission.BATTERY_STATS"},
	},
	{
		Name:        "WakeLock",
		BuildTag:    "goleo_wakelock",
		Permissions: []string{"android.permission.WAKE_LOCK"},
	},
	{
		Name:        "Vibration",
		BuildTag:    "goleo_vibration",
		Permissions: []string{"android.permission.VIBRATE"},
	},
	{
		Name:        "Sensors",
		BuildTag:    "goleo_sensors",
		Permissions: []string{"android.permission.BODY_SENSORS"},
		IOSUsageDescs: map[string]string{
			"NSMotionUsageDescription": "Access motion sensors for app features",
		},
	},
	{
		Name:        "Camera",
		BuildTag:    "goleo_camera",
		Permissions: []string{"android.permission.CAMERA"},
		IOSUsageDescs: map[string]string{
			"NSCameraUsageDescription": "Access camera for photo and barcode capture",
		},
	},
	{
		Name:        "Bluetooth",
		BuildTag:    "goleo_ble",
		Permissions: []string{"android.permission.BLUETOOTH_SCAN", "android.permission.BLUETOOTH_CONNECT"},
		IOSUsageDescs: map[string]string{
			"NSBluetoothAlwaysUsageDescription": "Access Bluetooth for peripheral communication",
		},
	},
	{
		Name:        "NFC",
		BuildTag:    "goleo_nfc",
		Permissions: []string{"android.permission.NFC"},
		IOSUsageDescs: map[string]string{
			"NFCReaderUsageDescription": "Access NFC for tag reading and writing",
		},
	},
	{
		Name:        "Background",
		BuildTag:    "goleo_background",
		Permissions: []string{"android.permission.FOREGROUND_SERVICE", "android.permission.POST_NOTIFICATIONS"},
	},
	{
		Name:        "Push",
		BuildTag:    "goleo_push",
		Permissions: []string{"android.permission.POST_NOTIFICATIONS"},
	},
}

var scanPatterns = []struct {
	Pattern *regexp.Regexp
	Feature string
	Source  string // "go" or "ts"
}{
	// Go: explicit RegisterXxx() calls
	{Pattern: regexp.MustCompile(`RegisterClipboard\(`), Feature: "Clipboard", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterDialogs\(`), Feature: "Dialogs", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterFS\(`), Feature: "FileSystem", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterGeolocation\(`), Feature: "Geolocation", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterBattery\(`), Feature: "Battery", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterWakeLock\(`), Feature: "WakeLock", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterCamera\(`), Feature: "Camera", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterBLE\(`), Feature: "Bluetooth", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterNFC\(`), Feature: "NFC", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterSensors\(`), Feature: "Sensors", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterVibration\(`), Feature: "Vibration", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterBackground\(`), Feature: "Background", Source: "go"},
	{Pattern: regexp.MustCompile(`RegisterPush\(`), Feature: "Push", Source: "go"},
	// Go: invoke strings containing feature names
	{Pattern: regexp.MustCompile(`"goleo:(clipboard|nfc|ble|geolocation|camera|fs|dialog|battery|wakelock|vibration|sensor|background|push)[A-Z"']`), Feature: "StringRef", Source: "go"},
	// TS: convenience module imports
	{Pattern: regexp.MustCompile(`@goleo/bridge/(clipboard|nfc|bluetooth|geolocation|camera|fs|dialog|battery|screen|vibration|sensor|background|push)`), Feature: "ImportRef", Source: "ts"},
	// TS: on() event listeners for feature events
	{Pattern: regexp.MustCompile(`on\('goleo:(nfc|ble|geolocation|camera|background|push|location|battery)`), Feature: "EventRef", Source: "ts"},
}

func featureForTag(tag string) *Feature {
	for _, f := range featureRegistry {
		if f.BuildTag == tag {
			return &f
		}
	}
	return nil
}

func tagForName(name string) string {
	for _, f := range featureRegistry {
		if f.Name == name {
			return f.BuildTag
		}
	}
	return ""
}

// detectFeatureUsage scans a project directory for feature usage
// and returns the set of build tags needed.
func detectFeatureUsage(projectDir string) ([]string, error) {
	detected := make(map[string]bool)

	// Scan .go files
	err := filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			skip := []string{".goleo", "node_modules", ".git", "frontend", "vendor"}
			for _, s := range skip {
				if d.Name() == s && path != projectDir {
					return filepath.SkipDir
				}
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".ts" && ext != ".tsx" && ext != ".vue" && ext != ".js" && ext != ".jsx" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		sourceType := "go"
		if ext != ".go" {
			sourceType = "ts"
		}
		for _, sp := range scanPatterns {
			if sp.Source != "go" && sp.Source != sourceType {
				continue
			}
			if sp.Source == "go" && sourceType != "go" {
				continue
			}
			matches := sp.Pattern.FindAllString(content, -1)
			for _, m := range matches {
				var name string
				switch sp.Feature {
				case "StringRef", "ImportRef", "EventRef":
					// Extract feature name from match
					for _, f := range featureRegistry {
						if strings.Contains(m, strings.ToLower(f.Name)) || strings.Contains(m, f.BuildTag[6:]) {
							name = f.Name
							break
						}
					}
				default:
					name = sp.Feature
				}
				if name != "" {
					if tag := tagForName(name); tag != "" {
						detected[tag] = true
					}
				}
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("scanning source for features: %w", err)
	}

	var tags []string
	for t := range detected {
		tags = append(tags, t)
	}
	return tags, nil
}
