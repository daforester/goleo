//go:build linux && !mobilebuild && goleo_cgo_webview

package runtime

/*
#cgo linux pkg-config: gtk+-3.0 webkit2gtk-4.0
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

// goleo_on_permission_request grants WebKit permission requests (camera,
// microphone, geolocation, notifications, ...). The minimal webview library
// Goleo embeds does not handle the "permission-request" signal, so on
// WebKitGTK these requests would otherwise block forever — hanging the GTK main
// loop — because nothing ever answers them. Granting lets the corresponding
// browser APIs (e.g. getUserMedia) resolve. The webview only ever loads the
// app's own trusted content, so blanket-granting is appropriate here.
static gboolean goleo_on_permission_request(WebKitWebView *web_view,
                                            WebKitPermissionRequest *request,
                                            gpointer user_data) {
    (void)web_view;
    (void)user_data;
    webkit_permission_request_allow(request);
    return TRUE; // handled; don't fall back to the default (deny) behaviour
}

// goleo_enable_permissions connects the handler to the WebKitWebView, which the
// webview library adds as the direct child of its GtkWindow.
static void goleo_enable_permissions(void *window_ptr) {
    if (window_ptr == NULL) {
        return;
    }
    GtkWidget *window = (GtkWidget *)window_ptr;
    if (!GTK_IS_BIN(window)) {
        return;
    }
    GtkWidget *child = gtk_bin_get_child(GTK_BIN(window));
    if (child == NULL || !WEBKIT_IS_WEB_VIEW(child)) {
        return;
    }
    g_signal_connect(WEBKIT_WEB_VIEW(child), "permission-request",
                     G_CALLBACK(goleo_on_permission_request), NULL);
}
*/
import "C"

import "unsafe"

// enableWebviewPermissions wires up auto-granting of WebKitGTK permission
// requests so browser-API fallbacks (camera, microphone, geolocation) resolve
// instead of blocking the webview. window is webview_go's GtkWindow pointer
// (from WebView.Window()); a nil pointer is ignored.
func enableWebviewPermissions(window unsafe.Pointer) {
	C.goleo_enable_permissions(window)
}
