//go:build windows

package dialogs

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// Win32 implementations of the dialogs that have a direct API equivalent:
// message boxes (user32 MessageBoxW), file open/save (comdlg32
// GetOpenFileNameW / GetSaveFileNameW) and folder selection (shell32
// SHBrowseForFolderW).
//
// These previously shelled out to `powershell -Command` with a generated
// WinForms script. That is the behaviour AV products flag hardest — an
// unsigned binary spawning powershell.exe to run a script it just built — and
// it got goleo's own CLI quarantined. Calling the API directly removes the
// child process, the script generation, and the string-escaping that went with
// it.
//
// platformShowPrompt still uses PowerShell: Win32 has no input-box primitive,
// and synthesising one needs an in-memory DLGTEMPLATE, which is more risk than
// the shell-out it would replace. See dialogs_windows.go.
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	procMessageBoxW          = user32.NewProc("MessageBoxW")
	procGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	procGetSaveFileNameW     = comdlg32.NewProc("GetSaveFileNameW")
	procSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	procCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
)

const (
	// MessageBox button sets and icons.
	mbOK              = 0x0
	mbOKCancel        = 0x1
	mbYesNoCancel     = 0x3
	mbYesNo           = 0x4
	mbIconError       = 0x10
	mbIconQuestion    = 0x20
	mbIconWarning     = 0x30
	mbIconInformation = 0x40

	// MessageBox return codes.
	idOK     = 1
	idCancel = 2
	idYes    = 6
	idNo     = 7

	// OPENFILENAME flags.
	ofnFileMustExist    = 0x00001000
	ofnPathMustExist    = 0x00000800
	ofnAllowMultiSelect = 0x00000200
	ofnExplorer         = 0x00080000
	ofnOverwritePrompt  = 0x00000002
	ofnNoChangeDir      = 0x00000008

	// SHBrowseForFolder flags: return only file-system dirs, modern UI.
	bifReturnOnlyFSDirs = 0x00000001
	bifNewDialogStyle   = 0x00000040

	// Multi-select can return many paths; this bounds the buffer.
	fileBufLen = 32 * 1024
	maxPathLen = 260
)

// openFileNameW mirrors OPENFILENAMEW. Field order and 64-bit padding must
// match the C layout — lStructSize is validated by the dialog.
type openFileNameW struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        uintptr
	DwReserved        uint32
	FlagsEx           uint32
}

type browseInfoW struct {
	HwndOwner      uintptr
	PidlRoot       uintptr
	PszDisplayName *uint16
	LpszTitle      *uint16
	UlFlags        uint32
	Lpfn           uintptr
	LParam         uintptr
	IImage         int32
}

func utf16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		// Only fails on an embedded NUL; treat as empty rather than aborting a
		// dialog over a stray byte in a caption.
		empty, _ := syscall.UTF16PtrFromString("")
		return empty
	}
	return p
}

func msgIconFlag(icon string) uintptr {
	switch icon {
	case "error":
		return mbIconError
	case "warning":
		return mbIconWarning
	case "question":
		return mbIconQuestion
	default:
		return mbIconInformation
	}
}

// msgButtonFlag mirrors psMsgButtons' mapping so behaviour is unchanged.
func msgButtonFlag(btns []string) uintptr {
	switch len(btns) {
	case 0, 1:
		return mbOK
	case 2:
		if strings.EqualFold(btns[0], "yes") {
			return mbYesNo
		}
		return mbOKCancel
	default:
		return mbYesNoCancel
	}
}

// msgResultName renders the return code as the same strings the WinForms
// DialogResult produced, so callers parsing the result keep working.
func msgResultName(code uintptr) string {
	switch code {
	case idCancel:
		return "Cancel"
	case idYes:
		return "Yes"
	case idNo:
		return "No"
	case idOK:
		return "OK"
	default:
		return "OK"
	}
}

func platformShowMessage(opts MessageBoxOptions) (string, error) {
	// MessageBoxW is modal and pumps its own message loop, so it must stay on
	// one OS thread for the duration.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ret, _, _ := procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(opts.Message))),
		uintptr(unsafe.Pointer(utf16Ptr(opts.Title))),
		msgButtonFlag(opts.Buttons)|msgIconFlag(opts.Icon),
	)
	if ret == 0 {
		// Matches the previous behaviour: a failed dialog reports OK rather
		// than surfacing an error to the frontend.
		return "OK", nil
	}
	return msgResultName(ret), nil
}

// win32Filter builds the doubly-NUL-terminated "Label\0patterns\0...\0\0"
// string GetOpenFileName expects, replacing the "|"-joined WinForms form.
func win32Filter(filters []FileFilter) []uint16 {
	var buf []uint16
	add := func(s string) {
		enc, err := syscall.UTF16FromString(s)
		if err != nil {
			return
		}
		buf = append(buf, enc...) // UTF16FromString already NUL-terminates
	}
	if len(filters) == 0 {
		add("All Files (*.*)")
		add("*.*")
	} else {
		for _, f := range filters {
			pat := strings.Join(f.Patterns, ";")
			add(fmt.Sprintf("%s (%s)", f.Name, pat))
			add(pat)
		}
	}
	return append(buf, 0) // final terminator closing the list
}

func newOpenFileName(opts FileDialogOptions, buf []uint16, flags uint32) (*openFileNameW, []uint16) {
	filter := win32Filter(opts.Filters)
	ofn := &openFileNameW{
		LpstrFilter:  &filter[0],
		NFilterIndex: 1,
		LpstrFile:    &buf[0],
		NMaxFile:     uint32(len(buf)),
		LpstrTitle:   utf16Ptr(opts.Title),
		Flags:        flags,
	}
	if opts.DefaultPath != "" {
		ofn.LpstrInitialDir = utf16Ptr(opts.DefaultPath)
	}
	ofn.LStructSize = uint32(unsafe.Sizeof(*ofn))
	// filter is returned so the caller keeps it alive for the call's duration.
	return ofn, filter
}

// parseMultiSelect decodes GetOpenFileName's multi-select result: a directory,
// then each file name, all NUL-separated. A single selection is returned as
// one full path instead, with no separator.
func parseMultiSelect(buf []uint16) []string {
	var parts []string
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] != 0 {
			continue
		}
		if i == start { // double NUL ends the list
			break
		}
		parts = append(parts, syscall.UTF16ToString(buf[start:i]))
		start = i + 1
	}
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 {
		return parts
	}
	dir := parts[0]
	out := make([]string, 0, len(parts)-1)
	for _, name := range parts[1:] {
		out = append(out, dir+string('\\')+name)
	}
	return out
}

func platformOpenFile(opts FileDialogOptions) ([]string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	buf := make([]uint16, fileBufLen)
	flags := uint32(ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir)
	if opts.Multiple {
		flags |= ofnAllowMultiSelect
	}
	ofn, filter := newOpenFileName(opts, buf, flags)

	ret, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(ofn)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(ofn)
	if ret == 0 {
		return nil, nil // cancelled
	}
	return parseMultiSelect(buf), nil
}

func platformSaveFile(opts FileDialogOptions) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	buf := make([]uint16, maxPathLen*2)
	ofn, filter := newOpenFileName(opts, buf, ofnExplorer|ofnPathMustExist|ofnOverwritePrompt|ofnNoChangeDir)

	ret, _, _ := procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(ofn)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(ofn)
	if ret == 0 {
		return "", nil // cancelled
	}
	return syscall.UTF16ToString(buf), nil
}

func platformSelectFolder(opts FileDialogOptions) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	display := make([]uint16, maxPathLen)
	bi := browseInfoW{
		PszDisplayName: &display[0],
		LpszTitle:      utf16Ptr(opts.Title),
		UlFlags:        bifReturnOnlyFSDirs | bifNewDialogStyle,
	}

	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	runtime.KeepAlive(bi)
	if pidl == 0 {
		return "", nil // cancelled
	}
	// The PIDL is shell-allocated; free it however this returns.
	defer procCoTaskMemFree.Call(pidl)

	path := make([]uint16, maxPathLen)
	if ok, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0]))); ok == 0 {
		return "", fmt.Errorf("dialogs: selected item is not a file-system folder")
	}
	return syscall.UTF16ToString(path), nil
}
