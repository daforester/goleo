// Permission auto-grant for the WebView2 backend.
//
// WebView2 raises a PermissionRequested event for camera/mic/geolocation/etc.
// With no handler it blocks (e.g. getUserMedia never resolves) waiting on a
// prompt nothing answers. A glaze-hosted window loads only the app's own trusted
// content, so the handler grants every request — the Windows analog of the Linux
// permission-request shim. Wired in the controller-completed path
// (kindPermissionRequested).
package glaze

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

// iCoreWebView2PermissionRequestedEventArgs vtable (IDL order); we only call
// PutState. State values: 0=Default, 1=Allow, 2=Deny.
type iPermArgsVtbl struct {
	iUnknownVtbl
	GetURI             uintptr
	GetPermissionKind  uintptr
	GetIsUserInitiated uintptr
	GetState           uintptr
	PutState           uintptr
}
type iPermArgs struct{ vtbl *iPermArgsVtbl }

func asPermArgs(p uintptr) *iPermArgs { return (*iPermArgs)(ptr(p)) }

func (i *iPermArgs) PutState(state uint32) {
	purego.SyscallN(i.vtbl.PutState, uintptr(unsafe.Pointer(i)), uintptr(state))
}
