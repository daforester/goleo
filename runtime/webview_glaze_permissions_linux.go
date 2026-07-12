//go:build linux && !mobilebuild && !goleo_cgo_webview

// Pure-Go (purego) WebKitGTK permission auto-grant for the glaze backend — the
// cgo-free analog of webview_permissions_linux.go (which is compiled out under
// goleo_glaze). glaze does not connect WebKitGTK's "permission-request" signal
// on Linux, so without this, camera/mic/geolocation requests from the app's
// own content would hang or be denied instead of resolving. The webview only
// ever loads the app's trusted content, so blanket-granting is appropriate.
//
// Status: NOT yet validated on real Linux hardware — exercised by
// .github/workflows/glaze-verify.yml (getUserMedia over a secure localhost
// origin under xvfb). Kept out of the default build (goleo_glaze is opt-in)
// until that passes.

package runtime

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

// dlopen mode: resolve now, and NEVER load a library that isn't already mapped
// (RTLD_NOLOAD). glaze has already loaded exactly one GTK/WebKit stack into the
// process; loading the *other* GTK major here would corrupt GObject's type
// system. NOLOAD returns a handle only for an already-present library.
const (
	rtldNow    = 0x0002
	rtldNoLoad = 0x0004
)

// enableGlazePermissions connects the auto-grant handler to the WebKitWebView
// that glaze added as the single child of its GtkWindow. window is
// glaze.WebView.Window() (a GtkWindow*). A nil pointer or any missing symbol is
// ignored — this is a best-effort enhancement, never fatal.
func enableGlazePermissions(window unsafe.Pointer) {
	if window == nil {
		return
	}

	// Detect which stack glaze loaded, without loading anything new.
	gtk4 := false
	gtk := dlopenExisting("libgtk-4.so.1")
	if gtk != 0 {
		gtk4 = true
	} else {
		gtk = dlopenExisting("libgtk-3.so.0")
	}
	if gtk == 0 {
		return
	}

	webkit := firstExisting(
		"libwebkitgtk-6.0.so.4",  // GTK4 stack
		"libwebkit2gtk-4.1.so.0", // GTK3 / libsoup3
		"libwebkit2gtk-4.0.so.37",
	)
	gobject := dlopenExisting("libgobject-2.0.so.0")
	if webkit == 0 || gobject == 0 {
		return
	}

	// Resolve the child accessor (GTK3 GtkBin vs GTK4 GtkWindow), the signal
	// connector, and the allow function.
	var getChild func(uintptr) uintptr
	if gtk4 {
		purego.RegisterLibFunc(&getChild, gtk, "gtk_window_get_child")
	} else {
		purego.RegisterLibFunc(&getChild, gtk, "gtk_bin_get_child")
	}
	var signalConnect func(instance uintptr, signal string, handler, data, destroy uintptr, flags int) uint64
	purego.RegisterLibFunc(&signalConnect, gobject, "g_signal_connect_data")
	var permissionAllow func(request uintptr)
	purego.RegisterLibFunc(&permissionAllow, webkit, "webkit_permission_request_allow")

	webview := getChild(uintptr(window))
	if webview == 0 {
		return
	}

	// gboolean (*)(WebKitWebView*, WebKitPermissionRequest*, gpointer). Grant and
	// return TRUE so WebKit does not fall back to its default (deny) handler. The
	// callback signature mirrors glaze's own script-message handler.
	handler := purego.NewCallback(func(_ uintptr, request uintptr, _ uintptr) uintptr {
		permissionAllow(request)
		return 1 // TRUE
	})
	signalConnect(webview, "permission-request", handler, 0, 0, 0)
}

func dlopenExisting(name string) uintptr {
	h, err := purego.Dlopen(name, rtldNow|rtldNoLoad)
	if err != nil {
		return 0
	}
	return h
}

func firstExisting(names ...string) uintptr {
	for _, n := range names {
		if h := dlopenExisting(n); h != 0 {
			return h
		}
	}
	return 0
}
