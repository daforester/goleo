//go:build windows

// Windows arm of the secure-context spike. WebView2's documented, cgo-free path
// to a portless secure origin is SetVirtualHostNameToFolderMapping over an
// https:// virtual host — https ⇒ secure context. We drive it through
// go-webview2's edge.Chromium (the exact dependency goleo already uses on
// Windows), so no fork is needed here: goleo's Windows wrapper would just reach
// the same API.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

func init() { runtime.LockOSThread() } // the WebView2 UI thread owns the message loop

const vhost = "goleo.assets" // https://goleo.assets/ — a secure origin, no port

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassEx = user32.NewProc("RegisterClassExW")
	procCreateWindowEx  = user32.NewProc("CreateWindowExW")
	procDefWindowProc   = user32.NewProc("DefWindowProcW")
	procGetMessage      = user32.NewProc("GetMessageW")
	procTranslateMsg    = user32.NewProc("TranslateMessage")
	procDispatchMsg     = user32.NewProc("DispatchMessageW")
	procShowWindow      = user32.NewProc("ShowWindow")
	procUpdateWindow    = user32.NewProc("UpdateWindow")
	procPostQuitMessage = user32.NewProc("PostQuitMessage")

	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHndl = kernel32.NewProc("GetModuleHandleW")
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type msgStruct struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

const (
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wsOverlappedWin   = 0x00CF0000
	swShow            = 5
	cwUseDefault      = 0x80000000
)

func wndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmClose:
		procPostQuitMessage.Call(0)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

func main() {
	fmt.Fprintln(os.Stderr, "[spike] Windows custom-origin secure-context probe (WebView2 virtual host)")

	// Serve the probe from a temp folder mapped to the https virtual host.
	tmp, err := os.MkdirTemp("", "goleo-scheme-spike")
	if err != nil {
		fmt.Println("RESULT: FAIL (Windows/WebView2) — mkdtemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)
	if err := os.WriteFile(filepath.Join(tmp, "index.html"), []byte(probeHTML), 0o644); err != nil {
		fmt.Println("RESULT: FAIL (Windows/WebView2) — write index.html:", err)
		os.Exit(1)
	}

	hInst, _, _ := procGetModuleHndl.Call(0)
	hInstance := windows.Handle(hInst)
	className, _ := windows.UTF16PtrFromString("goleo-scheme-spike")
	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   windows.NewCallback(wndProc),
		hInstance:     hInstance,
		lpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	title, _ := windows.UTF16PtrFromString("goleo scheme spike")
	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsOverlappedWin,
		cwUseDefault, cwUseDefault, 640, 480,
		0, 0, uintptr(hInstance), 0,
	)
	if hwnd == 0 {
		fmt.Println("RESULT: FAIL (Windows/WebView2) — CreateWindowEx returned NULL")
		os.Exit(1)
	}
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)

	done := false
	chromium := edge.NewChromium()
	chromium.DataPath = filepath.Join(tmp, "wv2data")
	chromium.MessageCallback = func(s string) {
		if done {
			return
		}
		done = true
		pass := reportResult("Windows/WebView2", s)
		procPostQuitMessage.Call(0)
		// Stash exit code; main exits after the loop drains.
		if pass {
			exitCode = 0
		} else {
			exitCode = 1
		}
	}

	if !chromium.Embed(hwnd) {
		fmt.Println("RESULT: FAIL (Windows/WebView2) — Chromium.Embed failed (WebView2 runtime installed?)")
		os.Exit(1)
	}
	chromium.Resize()

	v3 := chromium.GetICoreWebView2_3()
	if v3 == nil {
		fmt.Println("RESULT: FAIL (Windows/WebView2) — ICoreWebView2_3 unavailable (cannot map virtual host)")
		os.Exit(1)
	}
	if err := v3.SetVirtualHostNameToFolderMapping(vhost, tmp, edge.COREWEBVIEW2_HOST_RESOURCE_ACCESS_KIND_ALLOW); err != nil {
		fmt.Println("RESULT: FAIL (Windows/WebView2) — SetVirtualHostNameToFolderMapping:", err)
		os.Exit(1)
	}

	chromium.Navigate("https://" + vhost + "/index.html")

	// Message loop until the probe reports (MessageCallback posts WM_QUIT).
	var msg msgStruct
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 { // WM_QUIT
			break
		}
		procTranslateMsg.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMsg.Call(uintptr(unsafe.Pointer(&msg)))
	}

	if !done {
		fmt.Println("RESULT: FAIL (Windows/WebView2) — window closed before probe reported")
		os.Exit(1)
	}
	os.Exit(exitCode)
}

var exitCode = 1
