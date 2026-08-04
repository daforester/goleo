package cmd

import (
	"io/fs"
	"strings"
	"testing"
)

// The frontend is embedded in the Go library and served over http://127.0.0.1:<port>; the
// native shells load that URL. So the build must NOT also copy frontend/dist into the
// native project — it did, on both platforms, shipping a second copy of the whole frontend
// in every APK, AAB and .app that nothing ever read.
//
// This test is the other half of that removal: if a shell ever starts loading from native
// assets, it will find nothing there, so the reference and the copy have to come back
// together. Failing here says which one is missing.
func TestNativeShellsDoNotLoadFromBundledAssets(t *testing.T) {
	// Patterns that mean "read the UI out of the native bundle rather than over loopback".
	forbidden := map[string]string{
		"file:///android_asset": "Android asset URL",
		"getAssets(":            "Android AssetManager",
		"AssetManager":          "Android AssetManager",
		"loadFileURL":           "WKWebView file URL load",
		"Bundle.main.url":       "iOS bundle resource lookup",
		"Bundle.main.path":      "iOS bundle resource lookup",
	}

	roots := []string{"templates/android", "templates/android-dev", "templates/ios"}
	for _, root := range roots {
		err := fs.WalkDir(mobileTemplates, root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			switch {
			case strings.HasSuffix(path, ".java"),
				strings.HasSuffix(path, ".kt"),
				strings.HasSuffix(path, ".kts"),
				strings.HasSuffix(path, ".swift"):
			default:
				return nil
			}
			b, err := mobileTemplates.ReadFile(path)
			if err != nil {
				return err
			}
			for pattern, what := range forbidden {
				if strings.Contains(string(b), pattern) {
					t.Errorf("%s references %s (%q), but the build no longer copies "+
						"frontend/dist into the native project — either restore the copy or "+
						"load over http://127.0.0.1:<port> as the other shells do",
						path, what, pattern)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
}

// And the shells must load over loopback, so the embedded server is genuinely the source
// of the UI rather than this being a copy nobody noticed was unused.
func TestNativeShellsLoadOverLoopback(t *testing.T) {
	for _, path := range []string{
		"templates/android/app/src/main/java/com/goleo/app/MainActivity.java",
		"templates/ios/App/AppDelegate.swift",
	} {
		b, err := mobileTemplates.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if !strings.Contains(string(b), "127.0.0.1") && !strings.Contains(string(b), "localhost") {
			t.Errorf("%s does not load over a loopback URL; if the UI now comes from "+
				"somewhere else, the removed frontend/dist copy may be needed again", path)
		}
	}
}
