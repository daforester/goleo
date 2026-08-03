//go:build windows

package notify

import (
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Notifications go through Shell_NotifyIcon rather than by shelling out to
// `powershell -Command <generated WinRT script>`.
//
// The PowerShell approach worked, but an unsigned binary spawning powershell.exe
// with a script that reflectively loads WinRT types is among the most reliably
// flagged behaviours there is: AV products quarantine the *host* binary over it,
// which is exactly what happened to goleo's own published CLI. It also cost a
// process spawn per notification and inherited every quoting hazard of building
// a script out of caller-supplied strings — the same class of bug that broke
// Set-Clipboard for any text containing a space.
//
// Shell_NotifyIcon is plain user32/shell32: no child process, no script, no
// escaping. Windows 10+ renders these balloons as toasts and files them in
// Action Center, so what the user sees matches what the WinRT script produced.
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procLoadIconW        = user32.NewProc("LoadIconW")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	nimAdd    = 0x0
	nimDelete = 0x2

	nifIcon = 0x02
	nifTip  = 0x04
	nifInfo = 0x10

	niifInfo = 0x1

	idiApplication = 32512

	// (HWND)-3: parents the window to the message-only window manager, so it
	// is never shown, never in the taskbar, and receives no input.
	hwndMessage = ^uintptr(2)

	// How long the owning icon outlives the call. The balloon is torn down
	// with its icon, so the icon has to survive the display; by then Windows
	// has handed the toast to Action Center, so removing it afterwards does
	// not retract the notification, it just stops dead icons accumulating.
	iconLinger = 20 * time.Second
)

// notifyIconDataW mirrors NOTIFYICONDATAW. Field order and 64-bit padding have
// to match the C layout exactly, because the shell validates cbSize — Go's
// natural alignment of these types reproduces it (976 bytes on amd64).
type notifyIconDataW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeout         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

var (
	// Created once and reused: a window class is registered process-wide and
	// re-registering fails, and the shell needs a stable owner window to
	// associate the balloon with.
	ownerOnce sync.Once
	ownerHWnd uintptr
	ownerErr  error

	// Keeps the WndProc callback reachable for the process lifetime;
	// syscall.NewCallback allocations are never freed, so this is created once
	// alongside the window rather than per notification.
	wndProc uintptr

	// Each notification gets a distinct icon id so overlapping calls don't
	// clobber one another's balloon.
	idMu   sync.Mutex
	idNext uint32 = 1
)

func nextIconID() uint32 {
	idMu.Lock()
	defer idMu.Unlock()
	id := idNext
	idNext++
	return id
}

// ensureOwnerWindow creates the hidden message-only window that owns the icon.
func ensureOwnerWindow() (uintptr, error) {
	ownerOnce.Do(func() {
		className, err := syscall.UTF16PtrFromString("GoleoNotifySink")
		if err != nil {
			ownerErr = err
			return
		}
		hInstance, _, _ := procGetModuleHandleW.Call(0)
		wndProc = syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
			r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
			return r
		})

		var wc wndClassExW
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		wc.LpfnWndProc = wndProc
		wc.HInstance = hInstance
		wc.LpszClassName = className
		if atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
			ownerErr = fmt.Errorf("notify: RegisterClassEx failed: %w", callErr)
			return
		}

		hwnd, _, callErr := procCreateWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(className)),
			0, 0, 0, 0, 0,
			hwndMessage, 0, hInstance, 0,
		)
		if hwnd == 0 {
			ownerErr = fmt.Errorf("notify: CreateWindowEx failed: %w", callErr)
			return
		}
		ownerHWnd = hwnd
	})
	return ownerHWnd, ownerErr
}

// copyUTF16 writes s into dst as a NUL-terminated UTF-16 string, truncating to
// fit. The shell ignores an over-long field outright, so truncating keeps a
// long body visible instead of dropping it.
func copyUTF16(dst []uint16, s string) {
	encoded := syscall.StringToUTF16(s)
	if len(encoded) > len(dst) {
		encoded = encoded[:len(dst)]
		encoded[len(encoded)-1] = 0
	}
	copy(dst, encoded)
}

func platformNotify(title, body string) error {
	hwnd, err := ensureOwnerWindow()
	if err != nil {
		return err
	}

	icon, _, _ := procLoadIconW.Call(0, uintptr(uint16(idiApplication)))

	var data notifyIconDataW
	data.CbSize = uint32(unsafe.Sizeof(data))
	data.HWnd = hwnd
	data.UID = nextIconID()
	data.UFlags = nifIcon | nifInfo | nifTip
	data.HIcon = icon
	data.DwInfoFlags = niifInfo
	copyUTF16(data.SzTip[:], title)
	copyUTF16(data.SzInfoTitle[:], title)
	copyUTF16(data.SzInfo[:], body)

	if r, _, callErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&data))); r == 0 {
		return fmt.Errorf("notify: Shell_NotifyIcon failed: %w", callErr)
	}

	go func(d notifyIconDataW) {
		time.Sleep(iconLinger)
		procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&d)))
	}(data)

	return nil
}

func platformPermissionGranted() bool {
	return true
}

func platformRequestPermission() string {
	return "granted"
}
