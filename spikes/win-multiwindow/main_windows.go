//go:build windows

// Spike: can go-webview2 host TWO webview windows in ONE process on Windows,
// each on its own locked OS thread? Windows gives every thread its own message
// queue, so two `Run()` loops on two goroutines *may* coexist — which would
// make in-process multi-window (the D4 alternative to today's multi-process
// model) cheap on Windows, with no edge-layer single-loop rewrite. Each webview
// gets a distinct WebView2 user-data dir to avoid a shared-profile conflict.
//
// Run on a Windows desktop: `cd spikes/win-multiwindow && go run .`
// PASS = two independent windows appear; closing each ends its own goroutine.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	webview "github.com/jchv/go-webview2"
)

func window(title, html string, wg *sync.WaitGroup) {
	defer wg.Done()
	runtime.LockOSThread() // pin this window's message loop to one OS thread

	w := webview.NewWithOptions(webview.WebViewOptions{
		Debug:     true,
		AutoFocus: true,
		DataPath:  filepath.Join(os.TempDir(), "goleo-win-mw", title),
		WindowOptions: webview.WindowOptions{
			Title:  title,
			Width:  640,
			Height: 400,
			Center: true,
		},
	})
	defer w.Destroy()
	w.SetHtml(html)
	fmt.Println("[spike] opened:", title)
	w.Run() // blocks this thread until the window closes
	fmt.Println("[spike] closed:", title)
}

func main() {
	fmt.Println("[spike] opening two in-process windows (one process, two UI threads)...")
	var wg sync.WaitGroup
	wg.Add(2)
	go window("Goleo Window A", "<h1>Window A</h1><p>In-process multi-window test.</p>", &wg)
	go window("Goleo Window B", "<h1>Window B</h1><p>Second window, same process.</p>", &wg)
	wg.Wait()
	fmt.Println("[spike] both windows closed — PASS if both appeared and worked independently")
}
