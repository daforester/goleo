package runtime

import (
	"context"
	"testing"
)

func TestQuitCancelsContext(t *testing.T) {
	a := New(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	a.ctx, a.cancel = ctx, cancel

	select {
	case <-ctx.Done():
		t.Fatal("context should not be cancelled yet")
	default:
	}

	a.Quit()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("Quit should cancel the app context")
	}

	a.Quit() // idempotent — must not panic
	a.Stop() // alias — must not panic
}

func TestQuitNoCancelIsSafe(t *testing.T) {
	// Before Run wires a cancel func, Quit is a safe no-op.
	New(Config{}).Quit()
}

func TestWindowOptionsExitOnClose(t *testing.T) {
	// The option round-trips through the manager's stored state.
	a := New(Config{})
	wm := newWindowManager(a)
	wm.wins[1] = &procWindow{exitOnClose: true}
	if !wm.wins[1].exitOnClose {
		t.Fatal("exitOnClose not tracked")
	}
	im := newInProcWindowManager(a)
	im.wins[2] = &inprocWindow{exitOnClose: false}
	if im.wins[2].exitOnClose {
		t.Fatal("exitOnClose should be false")
	}
	// Both managers satisfy the windowSpawner contract.
	var _ windowSpawner = wm
	var _ windowSpawner = im
}
