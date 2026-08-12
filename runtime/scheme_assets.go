package runtime

import (
	"io/fs"
	"mime"
	"net/url"
	"path"
	"strings"
)

// defaultAssetScheme is the custom URL scheme used to serve embedded assets when
// Config.SchemeAssets is enabled and Config.AssetScheme is unset.
const defaultAssetScheme = "goleo"

// buildAssetServer returns a backend-agnostic resolver that maps a request path
// to bytes + content type from the embedded frontend FS, with SPA index.html
// fallback and the bridge token injected into the root document. It is handed to
// a webview backend that supports custom-scheme serving (see webview_glaze.go),
// letting a desktop window load its UI from a portless, secure custom origin
// (e.g. goleo://app/) instead of the loopback HTTP server.
func buildAssetServer(feFS fs.FS, token string) func(urlPath string) ([]byte, string, bool) {
	return func(urlPath string) ([]byte, string, bool) {
		name := assetName(urlPath)

		data, err := fs.ReadFile(feFS, name)
		if err != nil {
			// SPA fallback: an unknown, extension-less route serves index.html so
			// client-side routing works (mirrors the loopback static handler).
			if path.Ext(name) != "" {
				return nil, "", false
			}
			name = "index.html"
			data, err = fs.ReadFile(feFS, name)
			if err != nil {
				return nil, "", false
			}
		}

		if name == "index.html" {
			return injectToken(data, token), "text/html; charset=utf-8", true
		}
		ct := mime.TypeByExtension(path.Ext(name))
		if ct == "" {
			ct = "application/octet-stream"
		}
		return data, ct, true
	}
}

// assetName turns a request URL (full "goleo://app/foo.js" or a bare "/foo.js")
// into a clean embedded-FS path, defaulting to index.html for the root.
func assetName(urlPath string) string {
	p := urlPath
	if u, err := url.Parse(urlPath); err == nil && u.Path != "" {
		p = u.Path
	}
	p = strings.TrimPrefix(path.Clean("/"+p), "/")
	if p == "" || p == "." {
		return "index.html"
	}
	return p
}
