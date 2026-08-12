//go:build linux

// Linux arm of the secure-context spike. WebKitGTK, unlike WKWebView, exposes an
// explicit API to mark a custom scheme as a secure context:
// webkit_security_manager_register_uri_scheme_as_secure. And the scheme handler
// itself can be attached externally via purego (the same trick goleo's
// permission-grant shim already uses) — so Linux needs NO glaze fork.
//
// We let glaze create the window + web view, then walk from its GtkWindow to the
// WebKitWebView, grab its WebKitWebContext, register a "goleoapp://" scheme
// handler on it, mark the scheme secure, and load the probe. glaze's own Bind
// carries the report back.
package main

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/crgimenes/glaze"
	"github.com/ebitengine/purego"
)

func init() { runtime.LockOSThread() } // GTK is main-thread-only.

const schemeName = "goleoapp"

const (
	rtldNow    = 0x0002
	rtldNoLoad = 0x0004
)

// probeHTMLBytes must outlive the async GInputStream we hand to WebKit
// (g_memory_input_stream_new_from_data with a NULL destroy does not copy), so it
// is a process-lifetime global.
var probeHTMLBytes = []byte(probeHTML)

var (
	gotReport  string
	haveReport bool

	// Retained so purego never GCs the trampoline while WebKit still holds it.
	schemeCallback uintptr
)

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

// registerSecureScheme attaches a "goleoapp://" scheme handler to the web view's
// context and marks it secure. window is glaze.WebView.Window() (a GtkWindow*),
// which must already have the WebKitWebView as its child (call after SetSize).
// Returns false if any symbol/handle is missing.
func registerSecureScheme(window unsafe.Pointer) bool {
	if window == nil {
		return false
	}
	gtk4 := false
	gtk := dlopenExisting("libgtk-4.so.1")
	if gtk != 0 {
		gtk4 = true
	} else {
		gtk = dlopenExisting("libgtk-3.so.0")
	}
	webkit := firstExisting(
		"libwebkitgtk-6.0.so.4",
		"libwebkit2gtk-4.1.so.0",
		"libwebkit2gtk-4.0.so.37",
	)
	gio := firstExisting("libgio-2.0.so.0")
	gobject := dlopenExisting("libgobject-2.0.so.0")
	if gtk == 0 || webkit == 0 || gio == 0 || gobject == 0 {
		fmt.Fprintln(os.Stderr, "[spike] missing a required already-loaded library")
		return false
	}

	var getChild func(uintptr) uintptr
	if gtk4 {
		purego.RegisterLibFunc(&getChild, gtk, "gtk_window_get_child")
	} else {
		purego.RegisterLibFunc(&getChild, gtk, "gtk_bin_get_child")
	}
	var (
		getContext          func(uintptr) uintptr
		registerScheme      func(ctx uintptr, scheme string, cb, data, notify uintptr)
		getSecurityManager  func(uintptr) uintptr
		registerAsSecure    func(sm uintptr, scheme string)
		schemeRequestFinish func(req, stream uintptr, streamLen int64, contentType string)
		memInputStreamNew   func(data unsafe.Pointer, length int, destroy uintptr) uintptr
		gObjectUnref        func(uintptr)
	)
	purego.RegisterLibFunc(&getContext, webkit, "webkit_web_view_get_context")
	purego.RegisterLibFunc(&registerScheme, webkit, "webkit_web_context_register_uri_scheme")
	purego.RegisterLibFunc(&getSecurityManager, webkit, "webkit_web_context_get_security_manager")
	purego.RegisterLibFunc(&registerAsSecure, webkit, "webkit_security_manager_register_uri_scheme_as_secure")
	purego.RegisterLibFunc(&schemeRequestFinish, webkit, "webkit_uri_scheme_request_finish")
	purego.RegisterLibFunc(&memInputStreamNew, gio, "g_memory_input_stream_new_from_data")
	purego.RegisterLibFunc(&gObjectUnref, gobject, "g_object_unref")

	view := getChild(uintptr(window))
	if view == 0 {
		fmt.Fprintln(os.Stderr, "[spike] no WebKitWebView child on the GtkWindow")
		return false
	}
	ctx := getContext(view)
	if ctx == 0 {
		fmt.Fprintln(os.Stderr, "[spike] webkit_web_view_get_context returned NULL")
		return false
	}

	// void (*)(WebKitURISchemeRequest *request, gpointer user_data): serve the
	// probe page for any goleoapp:// URL.
	schemeCallback = purego.NewCallback(func(request uintptr, _ uintptr) uintptr {
		stream := memInputStreamNew(unsafe.Pointer(&probeHTMLBytes[0]), len(probeHTMLBytes), 0)
		schemeRequestFinish(request, stream, int64(len(probeHTMLBytes)), "text/html")
		gObjectUnref(stream)
		return 0
	})
	registerScheme(ctx, schemeName, schemeCallback, 0, 0)
	registerAsSecure(getSecurityManager(ctx), schemeName)
	return true
}

func main() {
	fmt.Fprintln(os.Stderr, "[spike] Linux custom-scheme secure-context probe (WebKitGTK)")

	wv, err := glaze.New(false)
	if err != nil {
		fmt.Println("RESULT: FAIL (Linux/WebKitGTK) — glaze.New:", err)
		os.Exit(1)
	}
	wv.SetTitle("goleo scheme spike")
	wv.SetSize(640, 480, glaze.HintNone) // attaches + shows the web view

	if !registerSecureScheme(wv.Window()) {
		fmt.Println("RESULT: FAIL (Linux/WebKitGTK) — could not register secure scheme handler")
		os.Exit(1)
	}

	if err := wv.Bind("report", func(s string) {
		gotReport = s
		haveReport = true
		wv.Terminate()
	}); err != nil {
		fmt.Println("RESULT: FAIL (Linux/WebKitGTK) — Bind:", err)
		os.Exit(1)
	}

	wv.Navigate(schemeName + "://app/index.html")
	wv.Run() // returns when the report handler calls Terminate

	if !haveReport {
		fmt.Println("RESULT: FAIL (Linux/WebKitGTK) — no probe report (scheme handler may not have fired)")
		os.Exit(1)
	}
	exitFromResult(reportResult("Linux/WebKitGTK", gotReport))
}
