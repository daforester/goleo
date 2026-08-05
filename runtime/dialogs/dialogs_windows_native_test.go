//go:build windows

package dialogs

import (
	"syscall"
	"testing"
	"unsafe"
)

// The dialogs themselves are modal and can't be driven in a test, so the parts
// that actually go wrong are covered instead: the struct layout the OS
// validates, and the buffer encoding/decoding either side of the call.

// GetOpenFileName rejects the call outright if lStructSize doesn't match the
// OS's idea of OPENFILENAMEW, and the documented x64 size is 152 bytes. A
// mis-ordered or mis-padded field shows up here rather than as a silent
// failure to open a dialog.
func TestOpenFileNameStructSize(t *testing.T) {
	if got, want := unsafe.Sizeof(openFileNameW{}), uintptr(152); got != want {
		t.Errorf("sizeof(OPENFILENAMEW) = %d, want %d — field order or padding is wrong", got, want)
	}
}

func TestBrowseInfoStructSize(t *testing.T) {
	// HWND + PIDL + 2 pointers + DWORD(+pad) + fn ptr + LPARAM + int(+pad).
	if got, want := unsafe.Sizeof(browseInfoW{}), uintptr(64); got != want {
		t.Errorf("sizeof(BROWSEINFOW) = %d, want %d", got, want)
	}
}

// win32Filter has to produce "Label\0patterns\0...\0" with a final extra NUL —
// the "|"-joined WinForms form the PowerShell version used is not what the
// Win32 API accepts.
func TestWin32FilterEncoding(t *testing.T) {
	decode := func(buf []uint16) []string {
		var out []string
		start := 0
		for i, c := range buf {
			if c != 0 {
				continue
			}
			if i == start {
				break
			}
			out = append(out, syscall.UTF16ToString(buf[start:i]))
			start = i + 1
		}
		return out
	}

	t.Run("defaults to all files", func(t *testing.T) {
		got := decode(win32Filter(nil))
		want := []string{"All Files (*.*)", "*.*"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("win32Filter(nil) = %q, want %q", got, want)
		}
	})

	t.Run("label and pattern pairs", func(t *testing.T) {
		got := decode(win32Filter([]FileFilter{
			{Name: "Images", Patterns: []string{"*.png", "*.jpg"}},
			{Name: "Text", Patterns: []string{"*.txt"}},
		}))
		want := []string{"Images (*.png;*.jpg)", "*.png;*.jpg", "Text (*.txt)", "*.txt"}
		if len(got) != len(want) {
			t.Fatalf("win32Filter = %q, want %q", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("entry %d = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("list is double-NUL terminated", func(t *testing.T) {
		buf := win32Filter(nil)
		if n := len(buf); n < 2 || buf[n-1] != 0 || buf[n-2] != 0 {
			t.Errorf("filter list must end in two NULs, got tail %v", buf[max(0, len(buf)-3):])
		}
	})
}

// GetOpenFileName returns one full path for a single selection, but a
// directory followed by bare file names when several are picked.
func TestParseMultiSelect(t *testing.T) {
	pack := func(parts ...string) []uint16 {
		var buf []uint16
		for _, p := range parts {
			enc, _ := syscall.UTF16FromString(p) // NUL-terminated
			buf = append(buf, enc...)
		}
		return append(buf, 0)
	}

	t.Run("single selection is a whole path", func(t *testing.T) {
		got := parseMultiSelect(pack(`C:\tmp\one.txt`))
		if len(got) != 1 || got[0] != `C:\tmp\one.txt` {
			t.Errorf("got %q, want one full path", got)
		}
	})

	t.Run("multi selection joins dir and names", func(t *testing.T) {
		got := parseMultiSelect(pack(`C:\tmp`, `a.txt`, `b.txt`))
		want := []string{`C:\tmp\a.txt`, `C:\tmp\b.txt`}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("empty buffer yields nothing", func(t *testing.T) {
		if got := parseMultiSelect(make([]uint16, 16)); got != nil {
			t.Errorf("got %q, want nil", got)
		}
	})
}

// The Win32 return codes have to map back to the same strings the WinForms
// DialogResult produced, or callers parsing the result break.
func TestMessageBoxMapping(t *testing.T) {
	for _, tc := range []struct {
		btns []string
		want uintptr
	}{
		{nil, mbOK},
		{[]string{"OK"}, mbOK},
		{[]string{"Yes", "No"}, mbYesNo},
		{[]string{"yes", "no"}, mbYesNo},
		{[]string{"OK", "Cancel"}, mbOKCancel},
		{[]string{"Yes", "No", "Cancel"}, mbYesNoCancel},
	} {
		if got := msgButtonFlag(tc.btns); got != tc.want {
			t.Errorf("msgButtonFlag(%q) = %#x, want %#x", tc.btns, got, tc.want)
		}
	}

	for code, want := range map[uintptr]string{idOK: "OK", idCancel: "Cancel", idYes: "Yes", idNo: "No"} {
		if got := msgResultName(code); got != want {
			t.Errorf("msgResultName(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestMessageBoxIconMapping(t *testing.T) {
	for icon, want := range map[string]uintptr{
		"error":   mbIconError,
		"warning": mbIconWarning,
		"":        mbIconInformation,
		"unknown": mbIconInformation,
	} {
		if got := msgIconFlag(icon); got != want {
			t.Errorf("msgIconFlag(%q) = %#x, want %#x", icon, got, want)
		}
	}
}
