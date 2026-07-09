//go:build !(android || ios) || goleo_nfc

package nfc

import "sync"

type NFCMessage struct {
	Records []NFCRecord `json:"records"`
}

type NFCRecord struct {
	Type      string `json:"type"`
	MediaType string `json:"mediaType"`
	Data      []byte `json:"data"`
}

// Provider is a native NFC backend. On mobile the shell registers one via
// SetProvider (android.nfc / CoreNFC) and delivers scanned tags through the
// bridge event channel. Desktop machines have no NFC reader API; Chrome on
// Android exposes Web NFC as a fallback.
type Provider interface {
	StartScan() error
	StopScan() error
	Write(message NFCMessage) error
}

var (
	providerMu sync.RWMutex
	provider   Provider
)

// eventSink, when set by RegisterNFC, forwards NFC events (e.g. scanned tags)
// to the frontend via the bridge. Native backends (the libnfc desktop scanner,
// mobile providers) call emit() to push a "nfc:tag" event.
var (
	sinkMu    sync.RWMutex
	eventSink func(event string, data any)
)

// SetEventSink registers the function used to deliver NFC events to the
// frontend. Safe to call with nil to clear it.
func SetEventSink(fn func(event string, data any)) {
	sinkMu.Lock()
	defer sinkMu.Unlock()
	eventSink = fn
}

func emit(event string, data any) {
	sinkMu.RLock()
	fn := eventSink
	sinkMu.RUnlock()
	if fn != nil {
		fn(event, data)
	}
}

func SetProvider(p Provider) {
	providerMu.Lock()
	defer providerMu.Unlock()
	provider = p
}

func getProvider() Provider {
	providerMu.RLock()
	defer providerMu.RUnlock()
	return provider
}

func StartScan() error {
	if p := getProvider(); p != nil {
		return p.StartScan()
	}
	return platformStartScan()
}

func StopScan() error {
	if p := getProvider(); p != nil {
		return p.StopScan()
	}
	return platformStopScan()
}

func Write(message NFCMessage) error {
	if p := getProvider(); p != nil {
		return p.Write(message)
	}
	return platformWrite(message)
}
