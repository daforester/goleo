//go:build mobilebuild

package runtime

type WebviewWindow struct{}

func NewWebviewWindow(cfg windowConfig) WebviewWindow {
	return WebviewWindow{}
}

func (win *WebviewWindow) Navigate(url string)  {}
func (win *WebviewWindow) SetTitle(title string) {}
func (win *WebviewWindow) SetSize(width, height int) {}
func (win *WebviewWindow) Eval(js string)        {}
func (win *WebviewWindow) Run()                  {}
func (win *WebviewWindow) Destroy()              {}
func (win *WebviewWindow) IsValid() bool         { return false }
