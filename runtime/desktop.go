//go:build !android && !ios

package runtime

// RegisterDesktopFeatures registers all host features that are available
// on desktop (Windows, macOS, Linux). On mobile this file is excluded at
// compile time, so no extra permissions are declared.
func RegisterDesktopFeatures(b *Bridge) {
	RegisterClipboard(b)
	RegisterDialogs(b)
	RegisterFS(b)
}
