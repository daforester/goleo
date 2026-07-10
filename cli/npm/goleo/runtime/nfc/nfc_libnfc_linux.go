//go:build linux && goleo_libnfc

package nfc

/*
#cgo pkg-config: libnfc
#include <nfc/nfc.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

// goleo_nfc_scan_once polls the first available NFC reader once for an
// ISO14443A tag. On success it writes the tag UID as an uppercase hex string
// into out and returns 1; returns 0 if no tag appeared within timeout_ms, or a
// negative value on error (no reader / init failure). All libnfc structs are
// handled here in C so cgo never models the library's ABI.
static int goleo_nfc_scan_once(int timeout_ms, char *out, int outsz) {
    if (outsz > 0) out[0] = '\0';

    nfc_context *ctx = NULL;
    nfc_init(&ctx);
    if (ctx == NULL) return -1;

    nfc_device *dev = nfc_open(ctx, NULL);
    if (dev == NULL) { nfc_exit(ctx); return -2; }
    if (nfc_initiator_init(dev) < 0) { nfc_close(dev); nfc_exit(ctx); return -3; }

    const nfc_modulation nm = { .nmt = NMT_ISO14443A, .nbr = NBR_106 };
    nfc_target nt;
    uint8_t period = (uint8_t)((timeout_ms + 149) / 150); // uiPeriod is in 150ms units
    if (period < 1) period = 1;

    int res = nfc_initiator_poll_target(dev, &nm, 1, 1, period, &nt);
    int rc = 0;
    if (res > 0) {
        size_t n = nt.nti.nai.szUidLen;
        int p = 0;
        for (size_t i = 0; i < n; i++) {
            if (p + 3 > outsz) break;
            p += snprintf(out + p, outsz - p, "%02X", nt.nti.nai.abtUid[i]);
        }
        rc = 1;
    }

    nfc_close(dev);
    nfc_exit(ctx);
    return rc;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"
)

var (
	scanMu   sync.Mutex
	scanStop chan struct{}
)

// platformStartScan begins polling the local NFC reader in the background,
// emitting an "nfc:tag" event ({ uid }) for each newly-seen tag. Errors (no
// reader) surface as an "nfc:error" event and the loop backs off.
func platformStartScan() error {
	scanMu.Lock()
	defer scanMu.Unlock()
	if scanStop != nil {
		return nil // already scanning
	}
	stop := make(chan struct{})
	scanStop = stop

	go func() {
		var last string
		buf := make([]byte, 64)
		for {
			select {
			case <-stop:
				return
			default:
			}
			rc := int(C.goleo_nfc_scan_once(C.int(500), (*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf))))
			switch {
			case rc == 1:
				uid := C.GoString((*C.char)(unsafe.Pointer(&buf[0])))
				if uid != "" && uid != last {
					last = uid
					Emit("nfc:tag", map[string]any{"uid": uid})
				}
			case rc < 0:
				Emit("nfc:error", map[string]any{"code": rc, "message": "no NFC reader available"})
				time.Sleep(2 * time.Second) // avoid a busy loop when no reader is present
			default:
				last = "" // no tag in field; let the same tag re-trigger next time
			}
		}
	}()
	return nil
}

func platformStopScan() error {
	scanMu.Lock()
	defer scanMu.Unlock()
	if scanStop != nil {
		close(scanStop)
		scanStop = nil
	}
	return nil
}

// platformWrite is not implemented: writing NDEF over libnfc is tag-type
// specific (Mifare/NTAG) and typically needs libfreefare.
func platformWrite(NFCMessage) error {
	return fmt.Errorf("nfc: %w — libnfc tag writing needs tag-specific NDEF handling (libfreefare)", errors.ErrUnsupported)
}
