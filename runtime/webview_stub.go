//go:build mobilebuild

package runtime

import "unsafe"

type WebviewWindow struct{ sess *nativeSession }

func (win *WebviewWindow) evaler() nativeEvaler         { return nil }
func (win *WebviewWindow) NativeHandle() unsafe.Pointer { return nil }

// webviewSupportsSchemeAssets: this backend does not (yet) serve the UI from a
// custom secure scheme, so Config.SchemeAssets falls back to the loopback server.
func webviewSupportsSchemeAssets() bool { return false }

func NewWebviewWindow(cfg windowConfig) WebviewWindow {
	return WebviewWindow{}
}

func (win *WebviewWindow) Navigate(url string)            {}
func (win *WebviewWindow) SetTitle(title string)          {}
func (win *WebviewWindow) SetSize(width, height int)      {}
func (win *WebviewWindow) Eval(js string)                 {}
func (win *WebviewWindow) Init(js string)                 {}
func (win *WebviewWindow) Bind(name string, fn any) error { return nil }
func (win *WebviewWindow) Run()                           {}
func (win *WebviewWindow) Destroy()                       {}
func (win *WebviewWindow) Dispatch(f func())              {}
func (win *WebviewWindow) Terminate()                     {}
func (win *WebviewWindow) IsValid() bool                  { return false }
