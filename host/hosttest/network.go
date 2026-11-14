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
	"strconv"
	"sync"
	"syscall"
)

// The range from which Listen, ListenPacket and a UDP Dial pick a port
// when asked for port 0: IANA's dynamic ports.
const (
	firstEphemeralPort = 49152
	lastEphemeralPort  = 65535
)

// errListenerClosed is what connect returns when the listener closes before
// its Accept takes the connection. Dial turns it into ECONNREFUSED.
var errListenerClosed = errors.New("hosttest: listener closed")

// key is a host and port, with the host as written.
type key struct {
	name string
	port int
}

// everyName reports whether k's host listens on every name.
func (k key) everyName() bool {
	switch k.name {
	case "", "0.0.0.0", "::":
		return true
	}
	return false
}

// bound is the key a socket on k is held under: the empty name for a
// socket on every name, and the name as written otherwise.
func (k key) bound() key {
	if k.everyName() {
		return key{port: k.port}
	}
	return k
}

func (k key) String() string {
	return net.JoinHostPort(k.name, strconv.Itoa(k.port))
}

func (k key) addr() net.Addr {
	return addr(k.String())
}

func (k key) udpAddr() net.Addr {
	return udpAddr(k.String())
}

// protocol returns "tcp" or "udp" for a network the Host serves, and ""
// for any other.
func protocol(network string) string {
	switch network {
	case "tcp", "tcp4", "tcp6":
		return "tcp"
	case "udp", "udp4", "udp6":
		return "udp"
	}
	return ""
}

// parse splits address into its host and port for a network of one of
// the protocols given.
func parse(network, address string, protocols ...string) (key, error) {
	p := protocol(network)
	for _, want := range protocols {
		if p == want {
			return parseAddress(address)
		}
	}
	return key{}, net.UnknownNetworkError(network)
}

// parseAddress splits address into its host and port. An empty port is
// port 0, as it is to package net. Ports are numbers; service names are
// not looked up.
func parseAddress(address string) (key, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return key{}, err
	}
	if port == "" {
		return key{name: host}, nil
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return key{}, &net.AddrError{Err: "invalid port", Addr: address}
	}
	return key{name: host, port: int(n)}, nil
}

// ports is one protocol's sockets, by the key each is bound to: TCP's
// listeners, or UDP's sockets. Each protocol has a table of its own, so
// TCP and UDP can bind the same port, as on a real network.
type ports[T comparable] struct {
	bound map[key]T   // by key.bound
	held  map[int]int // the number of sockets on each port
	next  int         // the port a bind to port 0 tries first
}

func newPorts[T comparable]() ports[T] {
	return ports[T]{
		bound: make(map[key]T),
		held:  make(map[int]int),
		next:  firstEphemeralPort,
	}
}

// bind returns k with its port picked, if it is 0, or fails with
// EADDRINUSE when k conflicts with a socket.
func (p *ports[T]) bind(k key) (key, error) {
	if k.port == 0 {
		port, ok := p.free(k.name)
		if !ok {
			return k, os.NewSyscallError("bind", syscall.EADDRINUSE)
		}
		k.port = port
	} else if p.inUse(k) {
		return k, os.NewSyscallError("bind", syscall.EADDRINUSE)
	}
	return k, nil
}

// add holds k, which bind returned, for v.
func (p *ports[T]) add(k key, v T) {
	p.bound[k.bound()] = v
	p.held[k.port]++
}

// remove frees k.
func (p *ports[T]) remove(k key) {
	delete(p.bound, k.bound())
	if p.held[k.port]--; p.held[k.port] == 0 {
		delete(p.held, k.port)
	}
}

// holder returns the socket that holds k: the one bound to that exact
// name, or else one bound to every name on its port. It returns the zero
// T when there is none.
func (p *ports[T]) holder(k key) T {
	if v, ok := p.bound[k.bound()]; ok {
		return v
	}
	return p.bound[key{port: k.port}]
}

// inUse reports whether a bind to k conflicts with a socket.
func (p *ports[T]) inUse(k key) bool {
	if k.everyName() {
		return p.held[k.port] > 0
	}
	_, named := p.bound[k]
	_, every := p.bound[key{port: k.port}]
	return named || every
}

// free picks a port in the ephemeral range that name can bind, starting
// after the last one picked, as a real network does.
func (p *ports[T]) free(name string) (int, bool) {
	for range lastEphemeralPort - firstEphemeralPort + 1 {
		port := p.next
		p.next++
		if p.next > lastEphemeralPort {
			p.next = firstEphemeralPort
		}
		if !p.inUse(key{name: name, port: port}) {
			return port, true
		}
	}
	return 0, false
}

// addr is the net.Addr of a Host's listener: its host as written and its
// port.
type addr string

func (addr) Network() string  { return "tcp" }
func (a addr) String() string { return string(a) }

// listener is the net.Listener that Host.Listen returns. Dial hands it the
// server end of a net.Pipe over conns, and Accept takes it.
type listener struct {
	host  *Host
	key   key
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func (l *listener) Accept() (net.Conn, error) {
	// A closed listener accepts nothing, even a Dial that is still waiting.
	select {
	case <-l.done:
		return nil, l.closedErr("accept")
	default:
	}
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, l.closedErr("accept")
	}
}

// Close stops the listener. Accept and every waiting Dial return at once.
// Closing it again returns an error wrapping net.ErrClosed, as closing a
// real listener twice does.
func (l *listener) Close() error {
	closed := false
	l.once.Do(func() {
		closed = true
		close(l.done)
		l.host.remove(l)
	})
	if !closed {
		return l.closedErr("close")
	}
	return nil
}

func (l *listener) Addr() net.Addr {
	return l.key.addr()
}

func (l *listener) closedErr(op string) error {
	return &net.OpError{Op: op, Net: "tcp", Addr: l.Addr(), Err: net.ErrClosed}
}

// reaches reports whether a Dial to k would reach l.
func (l *listener) reaches(k key) bool {
	if l.key.everyName() {
		return k.port == l.key.port
	}
	return k.bound() == l.key
}

// connect hands server, the far end of client, to Accept. If ctx is done
// or the listener closes first, it closes both ends and returns ctx's error
// or errListenerClosed.
func (l *listener) connect(ctx context.Context, client, server net.Conn) error {
	var err error
	select {
	case l.conns <- server:
		return nil
	case <-l.done:
		err = errListenerClosed
	case <-ctx.Done():
		err = ctx.Err()
	}
	_ = client.Close()
	_ = server.Close()
	return err
}

// conn is one end of a net.Pipe, reporting the listener's address as the
// server's local address and the client's remote one.
type conn struct {
	net.Conn
	local, remote net.Addr
}

func (c *conn) LocalAddr() net.Addr {
	if c.local != nil {
		return c.local
	}
	return c.Conn.LocalAddr()
}

func (c *conn) RemoteAddr() net.Addr {
	if c.remote != nil {
		return c.remote
	}
	return c.Conn.RemoteAddr()
}
