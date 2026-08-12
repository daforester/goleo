//go:build !darwin && !linux && !windows

package main

import (
	"fmt"
	"os"
)

// Windows/other: goleo drives WebView2 via jchv/go-webview2, and glaze's WebView2
// scheme support is an upstream TODO — so this glaze-API proof targets only the
// backends goleo actually consumes from glaze (macOS + Linux).
func main() {
	fmt.Println("RESULT: SKIP — glaze scheme-API proof targets darwin/linux (goleo's glaze backends)")
	os.Exit(0)
}
