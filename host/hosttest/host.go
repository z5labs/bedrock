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

// Host is a test's network, of net.Pipe connections over TCP and
// in-memory datagrams over UDP, and a filesystem in a directory from
// tb.TempDir(), confined with os.Root. It starts no goroutines, so build
// it inside the testing/synctest bubble that uses it.
//
// A Host is safe for concurrent use.
type Host struct {
	tb   testing.TB
	dir  string
	root *os.Root

	mu        sync.Mutex
	tcp       ports[*listener]
	udp       ports[*socket]
	listening map[key]chan struct{}
}

// Option configures a [Host] built by [New].
type Option func(*Host)

// New returns a Host whose directory comes from tb.TempDir(), so it is
// removed when the test ends. It is configured by opts, and reports misuse
// through tb.
//
// New starts no goroutines and makes no channels: Listen, ListenPacket,
// Dial and Listening make theirs when they are called. To use a Host with testing/synctest,
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
		tcp:       newPorts[*listener](),
		udp:       newPorts[*socket](),
		listening: make(map[key]chan struct{}),
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
// The network is "tcp", "tcp4" or "tcp6", which share one set of ports,
// apart from UDP's. Every error is a *net.OpError. The listener's Addr
// reports the host as written in address and the port it holds.
func (h *Host) Listen(network, address string) (net.Listener, error) {
	k, err := parse(network, address, "tcp")
	if err != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Err: err}
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	k, err = h.tcp.bind(k)
	if err != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Addr: k.addr(), Err: err}
	}

	l := &listener{
		host:  h,
		key:   k,
		conns: make(chan net.Conn),
		done:  make(chan struct{}),
	}
	h.tcp.add(k, l)

	for wk, ch := range h.listening {
		if l.reaches(wk) && !isClosed(ch) {
			close(ch)
		}
	}
	return l, nil
}

// Dial connects to the host and port of address over "tcp", "tcp4" or
// "tcp6", or over "udp", "udp4" or "udp6". Every error is a *net.OpError.
//
// Over TCP, Dial connects to the listener for the address: one bound to
// that exact name, or else one that listens on every name for that port.
// It fails with ECONNREFUSED when there is none. It never waits for a
// listener. A Dial that finds its listener returns once the listener's
// Accept takes the connection. If ctx is done first, Dial closes both ends
// of the connection and returns ctx's error.
//
// Over UDP, Dial returns a connected socket at once, whether or not a
// socket holds the address, as a real one does. The socket is bound to the
// name it dials, as written, and a port from IANA's dynamic range, as a
// socket dialled over loopback is bound to the loopback address. It sends
// only to the address it dialled, and takes datagrams only from that
// address, dropping every other. A write to an address no socket holds
// succeeds, and the socket's next read or write fails with ECONNREFUSED,
// as Linux reports the ICMP port unreachable that answers it. The package
// documentation says what else the socket does.
func (h *Host) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	k, err := parse(network, address, "tcp", "udp")
	if err != nil {
		return nil, &net.OpError{Op: "dial", Net: network, Err: err}
	}
	udp := protocol(network) == "udp"
	if err := ctx.Err(); err != nil {
		a := k.addr()
		if udp {
			a = k.udpAddr()
		}
		return nil, &net.OpError{Op: "dial", Net: network, Addr: a, Err: err}
	}
	if udp {
		return h.dialUDP(network, k)
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

// Listening returns a channel that is closed once a TCP listener exists
// that a Dial to address would reach. It never waits for a UDP socket: a
// test that needs one bound waits with synctest.Wait. A test receives from it before dialling, so
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
	return h.tcp.holder(k)
}

// remove forgets l, and every closed Listening channel that a Dial no
// longer reaches, so the next Listening call makes a new one.
func (h *Host) remove(l *listener) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.tcp.remove(l.key)
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
