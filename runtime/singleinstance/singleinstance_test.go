package singleinstance

import (
	"testing"
	"time"
)

func TestAddrDeterministic(t *testing.T) {
	if addrFor("myapp") != addrFor("myapp") {
		t.Error("addr should be stable for the same appID")
	}
	if addrFor("app-a") == addrFor("app-b") {
		t.Error("different appIDs should (almost always) differ")
	}
}

func TestAcquireAndForward(t *testing.T) {
	appID := t.Name() // distinctive port per test
	got := make(chan []string, 1)

	inst, primary, err := Acquire(appID, []string{"first"}, func(args []string) { got <- args })
	if err != nil || !primary {
		t.Fatalf("first Acquire: primary=%v err=%v", primary, err)
	}
	defer inst.Close()

	// A second Acquire must NOT become primary; it forwards its args.
	inst2, primary2, err := Acquire(appID, []string{"second", "--flag"}, nil)
	if err != nil {
		t.Fatalf("second Acquire err: %v", err)
	}
	if primary2 || inst2 != nil {
		t.Fatalf("second Acquire should be a secondary (primary=%v inst=%v)", primary2, inst2)
	}

	select {
	case args := <-got:
		if len(args) != 2 || args[0] != "second" || args[1] != "--flag" {
			t.Fatalf("primary got wrong forwarded args: %v", args)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("primary never received the forwarded args")
	}
}

func TestAcquireAfterClose(t *testing.T) {
	appID := t.Name()
	inst, primary, err := Acquire(appID, nil, nil)
	if err != nil || !primary {
		t.Fatalf("first Acquire: %v %v", primary, err)
	}
	inst.Close()
	// Give the OS a moment to release the port.
	time.Sleep(50 * time.Millisecond)
	inst2, primary2, err := Acquire(appID, nil, nil)
	if err != nil || !primary2 {
		t.Fatalf("after close, a new Acquire should become primary: %v %v", primary2, err)
	}
	inst2.Close()
}
