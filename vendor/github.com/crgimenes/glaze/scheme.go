package glaze

import "unsafe"

// This file adds a custom-URL-scheme handler API to glaze. It is the change
// proposed upstream (github.com/crgimenes/glaze) so that a host app can serve
// its assets from a portless, custom origin (e.g. "app://") that WebKit/WebView2
// still treat as a SECURE CONTEXT — the property a loopback http://127.0.0.1
// server provides today. Registering a scheme handler must happen before the web
// view is created (on macOS the WKWebViewConfiguration is copied at init), so it
// is an Option passed to NewWithOptions, not a method on the running WebView.

// SchemeRequest describes an incoming request for a registered custom scheme.
type SchemeRequest struct {
	// URL is the full request URL, e.g. "app://host/index.html".
	URL string
	// Method is the HTTP-style method ("GET", ...). Best-effort; may be empty on
	// backends that do not surface it.
	Method string
}

// SchemeResponse is what a SchemeHandler returns for a request. A nil response
// is treated as a 404.
type SchemeResponse struct {
	// Body is the response payload. It must remain valid until the handler
	// returns; backends copy or stream it as needed.
	Body []byte
	// MIMEType defaults to "application/octet-stream" when empty.
	MIMEType string
	// StatusCode defaults to 200 when zero. Honored on WebView2; best-effort on
	// WKWebView / WebKitGTK (which model a served resource, not a full HTTP
	// response) — a non-2xx there is surfaced as a load failure.
	StatusCode int
	// Headers are extra response headers (e.g. "Cache-Control"). Best-effort per
	// backend.
	Headers map[string]string
}

// SchemeHandler serves responses for one registered scheme. It is invoked on the
// UI thread; keep it fast (read from an in-memory FS), or hand back bytes you
// prepared earlier.
type SchemeHandler func(*SchemeRequest) *SchemeResponse

// schemeMIME returns a response's MIME type or the octet-stream default.
func schemeMIME(r *SchemeResponse) string {
	if r != nil && r.MIMEType != "" {
		return r.MIMEType
	}
	return "application/octet-stream"
}

// Options configures a web view created with NewWithOptions.
type Options struct {
	// Debug enables the platform web inspector / dev tools.
	Debug bool
	// Window, if non-nil, is an existing native window to embed into (a
	// GtkWindow* / NSWindow* / HWND, matching the platform), mirroring NewWindow.
	Window unsafe.Pointer
	// SchemeHandlers maps a scheme name (without "://", e.g. "app") to its
	// handler. Registered as a secure context where the platform allows it.
	// Because handlers must be installed before the web view is created, they
	// cannot be added later.
	SchemeHandlers map[string]SchemeHandler
}
