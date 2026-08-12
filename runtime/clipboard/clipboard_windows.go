//go:build windows

package clipboard

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// The clipboard is talked to directly through the Win32 API rather than by
// shelling out to `powershell -Command Set-Clipboard <text>`: powershell.exe
// re-parses its command line and strips the quoting exec adds, so any text
// containing a space arrived as multiple positional arguments and the write
// failed outright ("A positional parameter cannot be found that accepts
// argument ..."). Passing the payload as data through the API means no
// quoting, escaping or console-codepage question can arise.
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procSetClipboardData = user32.NewProc("SetClipboardData")

	procGlobalAlloc   = kernel32.NewProc("GlobalAlloc")
	procGlobalFree    = kernel32.NewProc("GlobalFree")
	procGlobalLock    = kernel32.NewProc("GlobalLock")
	procGlobalUnlock  = kernel32.NewProc("GlobalUnlock")
	procGlobalSize    = kernel32.NewProc("GlobalSize")
	procRtlMoveMemory = kernel32.NewProc("RtlMoveMemory")
)

const (
	cfUnicodeText = 13     // CF_UNICODETEXT
	gmemMoveable  = 0x0002 // GMEM_MOVEABLE, required for clipboard handles

	openAttempts = 20
	openBackoff  = 10 * time.Millisecond
)

// openClipboard retries: the clipboard is a global, single-owner resource and
// another application may hold it open for a moment.
func openClipboard() error {
	var last error
	for i := 0; i < openAttempts; i++ {
		r, _, callErr := procOpenClipboard.Call(0)
		if r != 0 {
			return nil
		}
		last = callErr
		time.Sleep(openBackoff)
	}
	return fmt.Errorf("clipboard: OpenClipboard failed, another application is holding the clipboard: %w", last)
}

func platformReadText() (string, error) {
	// OpenClipboard associates the clipboard with the calling thread, so the
	// goroutine must not migrate before CloseClipboard.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := openClipboard(); err != nil {
		return "", err
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		// No text on the clipboard (it may be empty or hold another format).
		return "", nil
	}
	locked, _, callErr := procGlobalLock.Call(h)
	if locked == 0 {
		return "", fmt.Errorf("clipboard: GlobalLock failed: %w", callErr)
	}
	defer procGlobalUnlock.Call(h)

	size, _, _ := procGlobalSize.Call(h)
	n := int(size) / 2
	if n == 0 {
		return "", nil
	}
	// Copy out of the clipboard's memory before it is unlocked/closed.
	buf := make([]uint16, n)
	procRtlMoveMemory.Call(uintptr(unsafe.Pointer(&buf[0])), locked, uintptr(n*2))
	return syscall.UTF16ToString(buf), nil
}

func platformWriteText(text string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := openClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()

	if r, _, callErr := procEmptyClipboard.Call(); r == 0 {
		return fmt.Errorf("clipboard: EmptyClipboard failed: %w", callErr)
	}

	// CF_UNICODETEXT is a NUL-terminated UTF-16 string. utf16.Encode rather
	// than syscall.UTF16FromString so an embedded NUL truncates (what every
	// other app puts on the clipboard) instead of failing the whole write.
	units := append(utf16.Encode([]rune(text)), 0)

	h, _, callErr := procGlobalAlloc.Call(gmemMoveable, uintptr(len(units)*2))
	if h == 0 {
		return fmt.Errorf("clipboard: GlobalAlloc failed: %w", callErr)
	}
	locked, _, callErr := procGlobalLock.Call(h)
	if locked == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("clipboard: GlobalLock failed: %w", callErr)
	}
	procRtlMoveMemory.Call(locked, uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)*2))
	procGlobalUnlock.Call(h)

	if r, _, callErr := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		// The handle is still ours only while SetClipboardData has not taken it.
		procGlobalFree.Call(h)
		return fmt.Errorf("clipboard: SetClipboardData failed: %w", callErr)
	}
	// On success the system owns h — freeing it here would corrupt the clipboard.
	return nil
}
