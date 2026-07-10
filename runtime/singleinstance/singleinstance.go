// Package singleinstance enforces a single running instance of an app and
// forwards a later launch's args to the primary (e.g. to focus a window or
// handle a deep link) instead of starting a second backend. Pure Go and
// cross-platform: it uses a per-app loopback address; the first instance to
// bind it is primary, and later instances connect and forward. An ACK
// handshake ensures an unrelated program on the same port is never mistaken
// for our primary.
package singleinstance

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"time"
)

const magic = "goleo-single-instance/1"

type message struct {
	Magic string   `json:"magic"`
	AppID string   `json:"appID"`
	Args  []string `json:"args"`
}

// Instance is the primary's handle; Close releases the lock.
type Instance struct {
	ln          net.Listener
	appID       string
	onSecondary func(args []string)
}

// addrFor derives a stable loopback address for appID in the ephemeral range.
func addrFor(appID string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(magic + ":" + appID))
	port := 49152 + int(h.Sum32()%16000)
	return fmt.Sprintf("127.0.0.1:%d", port)
}

// Acquire attempts to become the primary instance for appID. If it succeeds it
// returns (instance, true, nil) and begins listening. If another instance holds
// the lock, it forwards args to it and returns (nil, false, nil) — the caller
// should exit. On error the caller should start normally (single-instance best
// effort).
func Acquire(appID string, args []string, onSecondary func(args []string)) (*Instance, bool, error) {
	addr := addrFor(appID)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Something holds the address — try to forward to a goleo primary.
		if ferr := forward(addr, appID, args); ferr == nil {
			return nil, false, nil // secondary: forwarded, caller should exit
		}
		return nil, false, fmt.Errorf("single-instance: could not bind or forward %s: %w", addr, err)
	}
	inst := &Instance{ln: ln, appID: appID, onSecondary: onSecondary}
	go inst.serve()
	return inst, true, nil
}

func (i *Instance) serve() {
	for {
		conn, err := i.ln.Accept()
		if err != nil {
			return // listener closed
		}
		go i.handle(conn)
	}
}

func (i *Instance) handle(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	var msg message
	if err := json.NewDecoder(conn).Decode(&msg); err != nil {
		return
	}
	if msg.Magic != magic || msg.AppID != i.appID {
		return // not one of our instances
	}
	_, _ = conn.Write([]byte("ok"))
	if i.onSecondary != nil {
		i.onSecondary(msg.Args)
	}
}

func forward(addr, appID string, args []string) error {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := json.NewEncoder(conn).Encode(message{Magic: magic, AppID: appID, Args: args}); err != nil {
		return err
	}
	// Require an ACK so a non-goleo listener can't be mistaken for our primary.
	buf := make([]byte, 2)
	if _, err := conn.Read(buf); err != nil || string(buf) != "ok" {
		return errors.New("single-instance: no ack from primary")
	}
	return nil
}

// Close releases the single-instance lock.
func (i *Instance) Close() error {
	if i.ln != nil {
		return i.ln.Close()
	}
	return nil
}
