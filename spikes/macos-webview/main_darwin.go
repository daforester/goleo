//go:build darwin

// Spike: prove that on real macOS, a cgo-free (purego) binding can drive a
// WKWebView and complete a JS<->Go round-trip via WKScriptMessageHandler +
// evaluateJavaScript — the socket-free IPC mechanism the in-process
// hidden-master model depends on. Runs headless on a CI runner.
package main

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

func init() { runtime.LockOSThread() } // AppKit/WebKit must run on the main thread

var (
	webView  objc.ID
	msgCount int
	passed   bool
)

func sel(name string) objc.SEL   { return objc.RegisterName(name) }
func class(name string) objc.Class { return objc.GetClass(name) }

func cstr(s string) unsafe.Pointer {
	b := append([]byte(s), 0)
	return unsafe.Pointer(&b[0])
}

func nsString(s string) objc.ID {
	return objc.ID(class("NSString")).Send(sel("stringWithUTF8String:"), cstr(s))
}

// didReceive is the Go implementation of
// userContentController:didReceiveScriptMessage: — the JS -> Go bridge.
func didReceive(self objc.ID, _cmd objc.SEL, ucc objc.ID, message objc.ID) {
	msgCount++
	fmt.Fprintf(os.Stderr, "[spike] JS->Go message received (count=%d)\n", msgCount)
	if msgCount == 1 {
		// Go -> JS: evaluate JS that posts a *second* message back to us.
		js := "window.webkit.messageHandlers.external.postMessage('from-go')"
		webView.Send(sel("evaluateJavaScript:completionHandler:"), nsString(js), uintptr(0))
	} else {
		passed = true
	}
}

func main() {
	fmt.Fprintln(os.Stderr, "[spike] start")

	if _, err := purego.Dlopen("/System/Library/Frameworks/WebKit.framework/WebKit",
		purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		fmt.Println("RESULT: FAIL dlopen WebKit:", err)
		os.Exit(1)
	}

	// Autorelease pool so autoreleased NSStrings etc. are managed.
	pool := objc.ID(class("NSAutoreleasePool")).Send(sel("new"))
	_ = pool

	// NSApplication as accessory (no dock icon). We pump the run loop manually
	// rather than calling -run (which would block forever).
	app := objc.ID(class("NSApplication")).Send(sel("sharedApplication"))
	app.Send(sel("setActivationPolicy:"), 1) // NSApplicationActivationPolicyAccessory

	config := objc.ID(class("WKWebViewConfiguration")).Send(sel("new"))
	ucc := config.Send(sel("userContentController"))

	handlerClass, err := objc.RegisterClass(
		"GoleoScriptMessageHandler",
		class("NSObject"),
		nil, nil,
		[]objc.MethodDef{{Cmd: sel("userContentController:didReceiveScriptMessage:"), Fn: didReceive}},
	)
	if err != nil {
		fmt.Println("RESULT: FAIL RegisterClass:", err)
		os.Exit(1)
	}
	handler := objc.ID(handlerClass).Send(sel("new"))
	ucc.Send(sel("addScriptMessageHandler:name:"), handler, nsString("external"))

	// CGRect is 4 CGFloats (float64 on 64-bit) — tests purego struct-by-value args.
	type CGRect struct{ X, Y, W, H float64 }
	webView = objc.ID(class("WKWebView")).Send(sel("alloc")).
		Send(sel("initWithFrame:configuration:"), CGRect{0, 0, 400, 300}, config)

	html := `<html><body><script>window.webkit.messageHandlers.external.postMessage('loaded')</script></body></html>`
	webView.Send(sel("loadHTMLString:baseURL:"), nsString(html), objc.ID(0))

	// Pump the main run loop up to ~10s, waiting for the round-trip.
	rl := objc.ID(class("NSRunLoop")).Send(sel("currentRunLoop"))
	dateClass := class("NSDate")
	for i := 0; i < 100 && !passed; i++ {
		date := objc.ID(dateClass).Send(sel("dateWithTimeIntervalSinceNow:"), float64(0.1))
		rl.Send(sel("runUntilDate:"), date)
	}

	if passed {
		fmt.Println("RESULT: PASS — WKWebView drove a JS->Go->JS round-trip via purego (cgo-free)")
		os.Exit(0)
	}
	fmt.Printf("RESULT: FAIL — round-trip incomplete (msgCount=%d)\n", msgCount)
	os.Exit(1)
}
