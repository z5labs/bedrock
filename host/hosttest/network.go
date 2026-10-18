// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
)

// The range Listen picks a port from for port 0: IANA's dynamic ports.
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

// bound is the key a listener on k is held under: the empty name for a
// listener on every name, and the name as written otherwise.
func (k key) bound() key {
	if k.everyName() {
		return key{port: k.port}
	}
	return k
}

func (k key) addr() net.Addr {
	return addr(net.JoinHostPort(k.name, strconv.Itoa(k.port)))
}

// parse splits address into its host and port for a network the Host
// serves.
func parse(network, address string) (key, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return key{}, net.UnknownNetworkError(network)
	}
	return parseAddress(address)
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
