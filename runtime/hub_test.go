package runtime

import (
	"encoding/json"
	"testing"
	"time"
)

// attach registers a fake client on a server's hub and returns its send channel.
// It bypasses the real WebSocket upgrade — the concern here is which hub a client
// lands in, not the transport.
func attach(t *testing.T, s *Server) chan []byte {
	t.Helper()
	h := s.clients()
	c := &WSClient{send: make(chan []byte, 8), hub: h}
	h.register <- c

	// Wait for THIS client specifically. Waiting for "any client" is not enough: if
	// the hubs are shared (the bug under test) the first client already satisfies
	// that, so the second would race the broadcast and the test would pass for the
	// wrong reason — which is exactly what happened while writing this.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, got := range h.GetAll() {
			if got == c {
				return c.send
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("client never registered")
	return nil
}

func eventName(t *testing.T, raw []byte) string {
	t.Helper()
	var env struct {
		Type string `json:"type"`
		Data struct {
			Event string `json:"event"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("bad frame %s: %v", raw, err)
	}
	return env.Data.Event
}

// The hub was a package-level global started from init() and shared by every
// Server in the process, with clients registered against no particular server. So
// broadcastEvent fanned out to ALL of them: in a process running two Apps, app A's
// bridge events were delivered to app B's frontend.
func TestEventsDoNotLeakBetweenServers(t *testing.T) {
	srvA, err := NewServer(Config{Title: "A"}, NewBridge())
	if err != nil {
		t.Fatal(err)
	}
	srvB, err := NewServer(Config{Title: "B"}, NewBridge())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		srvA.clients().close()
		srvB.clients().close()
	})

	clientA := attach(t, srvA)
	clientB := attach(t, srvB)

	srvA.broadcastEvent(EventMessage{Event: "a:only"})

	select {
	case frame := <-clientA:
		if got := eventName(t, frame); got != "a:only" {
			t.Errorf("client A got event %q, want a:only", got)
		}
	case <-time.After(time.Second):
		t.Fatal("client A never received its own server's event")
	}

	select {
	case frame := <-clientB:
		t.Errorf("client B received server A's event %q — hubs are still shared", eventName(t, frame))
	case <-time.After(100 * time.Millisecond):
		// Correct: B heard nothing.
	}
}

// Each server's hub must go away with it, rather than leaking a goroutine that
// loops forever — run() previously had no exit at all.
func TestHubCloseReleasesClients(t *testing.T) {
	srv, err := NewServer(Config{Title: "X"}, NewBridge())
	if err != nil {
		t.Fatal(err)
	}
	send := attach(t, srv)

	srv.clients().close()

	// Closing the hub closes each client's send channel, which is what stops its
	// writePump.
	select {
	case _, open := <-send:
		if open {
			t.Error("expected the send channel to be closed")
		}
	case <-time.After(time.Second):
		t.Error("client send channel was not closed by hub shutdown")
	}
}

// close must be safe to call more than once — Stop can run twice (Quit is
// idempotent, and tests call it too).
func TestHubCloseIsIdempotent(t *testing.T) {
	srv, err := NewServer(Config{Title: "Y"}, NewBridge())
	if err != nil {
		t.Fatal(err)
	}
	h := srv.clients()
	h.close()
	h.close() // must not panic on a double close of the stop channel
}

// A Server built as a bare struct literal (as several tests do) must still get a
// working hub rather than nil-panicking.
func TestBareServerGetsAHub(t *testing.T) {
	s := &Server{}
	h := s.clients()
	if h == nil {
		t.Fatal("expected a hub")
	}
	t.Cleanup(h.close)
	if s.clients() != h {
		t.Error("clients() should return the same hub every time")
	}
}
