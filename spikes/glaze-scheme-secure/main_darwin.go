//go:build darwin

// macOS arm of the secure-context spike. This is the GATING platform: WKWebView
// has no public API to mark a custom scheme as a secure context (unlike Linux's
// webkit_security_manager_register_uri_scheme_as_secure and Windows' https
// virtual host). So we register a WKURLSchemeHandler for "goleoapp://", load the
// probe page from it, and let the page tell us whether WebKit granted it a
// secure context.
//
// Everything is cgo-free (purego + the Objective-C runtime), mirroring how
// glaze/goleo already drive WKWebView. If this reports PASS on real macOS, the
// same setURLSchemeHandler-before-init wiring can live in a glaze fork and the
// uniform all-platforms "goleo://" PR is viable.
package main

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

func init() { runtime.LockOSThread() } // AppKit/WebKit are main-thread-only.

const schemeName = "goleoapp"

var (
	gotReport string
	haveReport bool

	// Retained globally so ARC/autorelease never reclaims our delegates while the
	// web view still references them.
	schemeHandler objc.ID
	reportHandler objc.ID
)

func sel(name string) objc.SEL { return objc.RegisterName(name) }
func class(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

func cptr(s string) unsafe.Pointer {
	b := append([]byte(s), 0)
	return unsafe.Pointer(&b[0])
}

func nsString(s string) objc.ID {
	return class("NSString").Send(sel("stringWithUTF8String:"), cptr(s))
}

// goString reads a NUL-terminated C string returned by -UTF8String.
func goString(id objc.ID) string {
	if id == 0 {
		return ""
	}
	p := *(*unsafe.Pointer)(unsafe.Pointer(&id))
	var n int
	for *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(p), n))
}

// startURLSchemeTask implements -webView:startURLSchemeTask: — serve the probe
// page for any goleoapp:// URL.
func startURLSchemeTask(self objc.ID, _cmd objc.SEL, webView objc.ID, task objc.ID) {
	body := []byte(probeHTML)
	req := task.Send(sel("request"))
	url := req.Send(sel("URL"))

	data := class("NSData").Send(sel("dataWithBytes:length:"), unsafe.Pointer(&body[0]), len(body))
	resp := class("NSURLResponse").Send(sel("alloc")).Send(
		sel("initWithURL:MIMEType:expectedContentLength:textEncodingName:"),
		url, nsString("text/html"), len(body), nsString("UTF-8"))

	task.Send(sel("didReceiveResponse:"), resp)
	task.Send(sel("didReceiveData:"), data)
	task.Send(sel("didFinish"))
}

// stopURLSchemeTask implements -webView:stopURLSchemeTask: (nothing to cancel —
// we complete synchronously).
func stopURLSchemeTask(self objc.ID, _cmd objc.SEL, webView objc.ID, task objc.ID) {}

// didReceiveScriptMessage implements the "report" WKScriptMessageHandler.
func didReceiveScriptMessage(self objc.ID, _cmd objc.SEL, ucc objc.ID, message objc.ID) {
	gotReport = goString(message.Send(sel("body")).Send(sel("UTF8String")))
	haveReport = true
}

func main() {
	fmt.Fprintln(os.Stderr, "[spike] macOS custom-scheme secure-context probe")

	if _, err := purego.Dlopen("/System/Library/Frameworks/WebKit.framework/WebKit",
		purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		fmt.Println("RESULT: FAIL (macOS/WKWebView) — dlopen WebKit:", err)
		os.Exit(1)
	}
	pool := class("NSAutoreleasePool").Send(sel("new"))
	_ = pool

	app := class("NSApplication").Send(sel("sharedApplication"))
	app.Send(sel("setActivationPolicy:"), 1) // accessory (no dock icon)

	// --- register the delegate classes -------------------------------------
	schemeClass, err := objc.RegisterClass(
		"GoleoURLSchemeHandler", objc.GetClass("NSObject"),
		[]*objc.Protocol{objc.GetProtocol("WKURLSchemeHandler")}, nil,
		[]objc.MethodDef{
			{Cmd: sel("webView:startURLSchemeTask:"), Fn: startURLSchemeTask},
			{Cmd: sel("webView:stopURLSchemeTask:"), Fn: stopURLSchemeTask},
		})
	if err != nil {
		fmt.Println("RESULT: FAIL (macOS/WKWebView) — RegisterClass scheme handler:", err)
		os.Exit(1)
	}
	msgClass, err := objc.RegisterClass(
		"GoleoReportHandler", objc.GetClass("NSObject"),
		[]*objc.Protocol{objc.GetProtocol("WKScriptMessageHandler")}, nil,
		[]objc.MethodDef{
			{Cmd: sel("userContentController:didReceiveScriptMessage:"), Fn: didReceiveScriptMessage},
		})
	if err != nil {
		fmt.Println("RESULT: FAIL (macOS/WKWebView) — RegisterClass report handler:", err)
		os.Exit(1)
	}

	// --- build the configuration BEFORE the WKWebView (config is copied at
	//     init, so the scheme handler MUST be set here) ----------------------
	config := class("WKWebViewConfiguration").Send(sel("new"))
	schemeHandler = objc.ID(schemeClass).Send(sel("new"))
	config.Send(sel("setURLSchemeHandler:forURLScheme:"), schemeHandler, nsString(schemeName))

	ucc := config.Send(sel("userContentController"))
	reportHandler = objc.ID(msgClass).Send(sel("new"))
	ucc.Send(sel("addScriptMessageHandler:name:"), reportHandler, nsString("report"))

	type cgRect struct{ X, Y, W, H float64 }
	webView := class("WKWebView").Send(sel("alloc")).Send(
		sel("initWithFrame:configuration:"), cgRect{0, 0, 480, 360}, config)

	// --- load the probe from the custom origin -----------------------------
	url := class("NSURL").Send(sel("URLWithString:"), nsString(schemeName+"://app/index.html"))
	req := class("NSURLRequest").Send(sel("requestWithURL:"), url)
	webView.Send(sel("loadRequest:"), req)

	// Pump the main run loop up to ~15s waiting for the probe report.
	rl := class("NSRunLoop").Send(sel("currentRunLoop"))
	dateClass := class("NSDate")
	for i := 0; i < 150 && !haveReport; i++ {
		date := dateClass.Send(sel("dateWithTimeIntervalSinceNow:"), float64(0.1))
		rl.Send(sel("runUntilDate:"), date)
	}

	if !haveReport {
		fmt.Println("RESULT: FAIL (macOS/WKWebView) — no probe report within timeout (scheme handler may not have fired)")
		os.Exit(1)
	}
	exitFromResult(reportResult("macOS/WKWebView", gotReport))
}
