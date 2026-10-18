// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
)

// Host is a test's network, of net.Pipe connections, and a filesystem in
// a directory from tb.TempDir(), confined with os.Root. It starts no
// goroutines, so build it inside the testing/synctest bubble that uses it.
//
// A Host is safe for concurrent use.
type Host struct {
	tb   testing.TB
	dir  string
	root *os.Root

	mu        sync.Mutex
	listeners map[key]*listener
	held      map[int]int // the number of listeners on each port
	listening map[key]chan struct{}
	nextPort  int
}

// Option configures a [Host] built by [New].
type Option func(*Host)

// New returns a Host whose directory comes from tb.TempDir(), so it is
// removed when the test ends. It is configured by opts, and reports misuse
// through tb.
//
// New starts no goroutines and makes no channels: Listen and Listening
// make theirs when they are called. To use a Host with testing/synctest,
// call New inside synctest.Test with the bubble's t, so that every
// channel the Host makes belongs to the bubble that uses it.
func New(tb testing.TB, opts ...Option) *Host {
	tb.Helper()
	dir := tb.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		tb.Fatalf("hosttest: New: %v", err)
	}
	// Cleanups run last in, first out, so the root closes before
	// tb.TempDir's cleanup removes the directory.
	tb.Cleanup(func() { _ = root.Close() })

	h := &Host{
		tb:        tb,
		dir:       dir,
		root:      root,
		listeners: make(map[key]*listener),
		held:      make(map[int]int),
		listening: make(map[key]chan struct{}),
		nextPort:  firstEphemeralPort,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Listen listens on the host and port of address. An empty host, 0.0.0.0
// or :: listens on every name. Port 0 picks a free port. Listen fails with
// EADDRINUSE when a listener already holds the same name and port, or
// when one of the two listens on every name for that port.
//
// The network is "tcp", "tcp4" or "tcp6", which share one set of ports.
// Every error is a *net.OpError. The listener's Addr reports the host as
// written in address and the port it holds.
func (h *Host) Listen(network, address string) (net.Listener, error) {
	k, err := parse(network, address)
	if err != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Err: err}
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if k.port == 0 {
		port, ok := h.freePortLocked(k.name)
		if !ok {
			return nil, &net.OpError{Op: "listen", Net: network, Addr: k.addr(), Err: os.NewSyscallError("bind", syscall.EADDRINUSE)}
		}
		k.port = port
	} else if h.inUseLocked(k) {
		return nil, &net.OpError{Op: "listen", Net: network, Addr: k.addr(), Err: os.NewSyscallError("bind", syscall.EADDRINUSE)}
	}

	l := &listener{
		host:  h,
		key:   k,
		conns: make(chan net.Conn),
		done:  make(chan struct{}),
	}
	h.listeners[k.bound()] = l
	h.held[k.port]++

	for wk, ch := range h.listening {
		if l.reaches(wk) && !isClosed(ch) {
			close(ch)
		}
	}
	return l, nil
}

// Dial connects to the listener for the host and port of address: one
// bound to that exact name, or else one that listens on every name for
// that port. It fails with ECONNREFUSED when there is none. It never
// waits for a listener.
//
// A Dial that finds its listener returns once the listener's Accept takes
// the connection. If ctx is done first, Dial closes both ends of the
// connection and returns ctx's error. Every error is a *net.OpError.
func (h *Host) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	k, err := parse(network, address)
	if err != nil {
		return nil, &net.OpError{Op: "dial", Net: network, Err: err}
	}
	if err := ctx.Err(); err != nil {
		return nil, &net.OpError{Op: "dial", Net: network, Addr: k.addr(), Err: err}
	}

	h.mu.Lock()
	l := h.reachableLocked(k)
	h.mu.Unlock()
	if l == nil {
		return nil, refused(network, k)
	}

	client, server := net.Pipe()
	if err := l.connect(ctx, client, &conn{Conn: server, local: l.Addr()}); err != nil {
		if errors.Is(err, errListenerClosed) {
			return nil, refused(network, k)
		}
		return nil, &net.OpError{Op: "dial", Net: network, Addr: k.addr(), Err: err}
	}
	return &conn{Conn: client, remote: k.addr()}, nil
}

// Listening returns a channel that is closed once a listener exists that a
// Dial to address would reach. A test receives from it before dialling, so
// that it doesn't dial before the App listens. It works like bedrocktest's
// Process.Notified: once closed, the channel stays closed, and after that
// listener closes, Listening returns a new channel, closed when the next
// one is made. The channel is created on the first call, and no goroutine
// is started, so Listening works inside a testing/synctest bubble.
//
// If address is not a host and port, or its port is 0, which no listener
// holds, Listening fails the test through the testing.TB given to New and
// returns a closed channel, so that the test goes on to report the mistake
// rather than hang.
func (h *Host) Listening(address string) <-chan struct{} {
	k, err := parseAddress(address)
	if err == nil && k.port == 0 {
		err = errors.New("port 0 is never listened on; wait for the port the listener picked")
	}
	if err != nil {
		h.tb.Helper()
		h.tb.Errorf("hosttest: Listening(%q): %v", address, err)
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if ch, ok := h.listening[k]; ok {
		return ch
	}
	ch := make(chan struct{})
	h.listening[k] = ch
	if h.reachableLocked(k) != nil {
		close(ch)
	}
	return ch
}

// reachableLocked returns the listener a Dial to k reaches, or nil.
func (h *Host) reachableLocked(k key) *listener {
	if l, ok := h.listeners[k.bound()]; ok {
		return l
	}
	return h.listeners[key{port: k.port}]
}

// inUseLocked reports whether a Listen on k conflicts with a listener.
func (h *Host) inUseLocked(k key) bool {
	if k.everyName() {
		return h.held[k.port] > 0
	}
	_, named := h.listeners[k]
	_, every := h.listeners[key{port: k.port}]
	return named || every
}

// freePortLocked picks a port in the ephemeral range that name can listen
// on, starting after the last one picked, as a real network does.
func (h *Host) freePortLocked(name string) (int, bool) {
	for range lastEphemeralPort - firstEphemeralPort + 1 {
		port := h.nextPort
		h.nextPort++
		if h.nextPort > lastEphemeralPort {
			h.nextPort = firstEphemeralPort
		}
		if !h.inUseLocked(key{name: name, port: port}) {
			return port, true
		}
	}
	return 0, false
}

// remove forgets l, and every closed Listening channel that a Dial no
// longer reaches, so the next Listening call makes a new one.
func (h *Host) remove(l *listener) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.listeners, l.key.bound())
	if h.held[l.key.port]--; h.held[l.key.port] == 0 {
		delete(h.held, l.key.port)
	}
	for wk, ch := range h.listening {
		if isClosed(ch) && h.reachableLocked(wk) == nil {
			delete(h.listening, wk)
		}
	}
}

func refused(network string, k key) error {
	return &net.OpError{Op: "dial", Net: network, Addr: k.addr(), Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
}

func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
