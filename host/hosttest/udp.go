// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest

import (
	"bytes"
	"net"
	"net/netip"
	"os"
	"sync"
	"syscall"
	"time"
)

// The largest datagram UDP carries over loopback: 65,535 bytes less the
// IPv4 and UDP headers, or, to an IPv6 address, less the UDP header,
// whose IPv6 payload length leaves out the IPv6 header.
const (
	maxDatagram4 = 65535 - 20 - 8
	maxDatagram6 = 65535 - 8
)

// ListenPacket binds a UDP socket to the host and port of address. An
// empty host, 0.0.0.0 or :: binds every name. Port 0 picks a free port.
// ListenPacket fails with EADDRINUSE when a socket already holds the same
// name and port, or when one of the two binds every name for that port.
//
// The network is "udp", "udp4" or "udp6", which share one set of ports,
// apart from TCP's, so a UDP socket and a TCP listener can hold the same
// port. Every error is a *net.OpError. The socket's LocalAddr reports the
// host as written in address and the port it holds.
//
// The socket takes every datagram sent to an address it holds, and the
// package documentation says what else it does. Like every socket the
// Host makes, it starts no goroutine; a test that needs it bound before
// the test sends to it waits with synctest.Wait.
func (h *Host) ListenPacket(network, address string) (net.PacketConn, error) {
	k, err := parse(network, address, "udp")
	if err != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Err: err}
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	k, err = h.udp.bind(k)
	if err != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Addr: k.udpAddr(), Err: err}
	}
	s := newSocket(h, network, k)
	h.udp.add(k, s)
	return packetConn{s}, nil
}

// dialUDP binds a socket to remote's name and a free port, connected to
// remote.
func (h *Host) dialUDP(network string, remote key) (net.Conn, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	local, err := h.udp.bind(key{name: remote.name})
	if err != nil {
		return nil, &net.OpError{Op: "dial", Net: network, Addr: remote.udpAddr(), Err: err}
	}
	s := newSocket(h, network, local)
	s.dialled, s.remote = true, remote
	h.udp.add(local, s)
	return udpConn{s}, nil
}

// deliver hands d to the socket that holds d.to, and reports whether one
// took it.
func (h *Host) deliver(d datagram) bool {
	h.mu.Lock()
	s := h.udp.holder(d.to)
	h.mu.Unlock()
	return s != nil && s.enqueue(d)
}

// removeSocket frees the port s holds.
func (h *Host) removeSocket(s *socket) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.udp.remove(s.local)
}

// maxDatagram is the largest datagram a socket can send to name: IPv6's
// limit for an IPv6 address, and IPv4's for every other name, which is
// never resolved.
func maxDatagram(name string) int {
	if a, err := netip.ParseAddr(name); err == nil && a.Is6() && !a.Is4In6() {
		return maxDatagram6
	}
	return maxDatagram4
}

// udpAddr is the net.Addr of a Host's UDP socket: its host as written and
// its port.
type udpAddr string

func (udpAddr) Network() string  { return "udp" }
func (a udpAddr) String() string { return string(a) }

// datagram is one datagram in flight: its bytes, the address it is from,
// and the address it was sent to.
type datagram struct {
	b        []byte
	from, to key
}

// socket is a UDP socket of a Host. ListenPacket returns it as a
// packetConn, and Dial as a udpConn.
type socket struct {
	host    *Host
	network string
	local   key  // as written, with the port it holds
	remote  key  // the address it dialled
	dialled bool // whether Dial made it

	ready  chan struct{} // holds a token when a read may have something to return
	done   chan struct{}
	once   sync.Once
	rd, wd deadline

	mu        sync.Mutex
	queue     []datagram
	err       error       // the error a dialled socket's next read or write returns
	replyFrom map[key]key // on every name, the address each peer last sent to
}

func newSocket(h *Host, network string, local key) *socket {
	s := &socket{
		host:      h,
		network:   network,
		local:     local,
		ready:     make(chan struct{}, 1),
		done:      make(chan struct{}),
		replyFrom: make(map[key]key),
	}
	s.rd.cancel = make(chan struct{})
	s.wd.cancel = make(chan struct{})
	return s
}

// recv waits for a datagram and copies it into b, truncating it if b is
// short. It returns the datagram's length in b and the address it is from.
// The error is the bare cause, for the caller to wrap.
func (s *socket) recv(b []byte) (int, key, error) {
	for {
		select {
		case <-s.done:
			return 0, key{}, net.ErrClosed
		case <-s.rd.wait():
			return 0, key{}, os.ErrDeadlineExceeded
		default:
		}

		s.mu.Lock()
		if err := s.err; err != nil {
			s.err = nil
			s.signalLocked()
			s.mu.Unlock()
			return 0, key{}, os.NewSyscallError("read", err)
		}
		if len(s.queue) > 0 {
			d := s.queue[0]
			s.queue[0] = datagram{}
			s.queue = s.queue[1:]
			s.signalLocked()
			s.mu.Unlock()
			return copy(b, d.b), d.from, nil
		}
		s.mu.Unlock()

		select {
		case <-s.ready:
		case <-s.done:
		case <-s.rd.wait():
		}
	}
}

// send sends b to to, as one datagram. call names the system call a real
// socket fails in. The error is the bare cause, for the caller to wrap.
func (s *socket) send(b []byte, to key, call string) error {
	select {
	case <-s.done:
		return net.ErrClosed
	case <-s.wd.wait():
		return os.ErrDeadlineExceeded
	default:
	}

	s.mu.Lock()
	if err := s.err; err != nil {
		s.err = nil
		s.mu.Unlock()
		return os.NewSyscallError(call, err)
	}
	from := s.local
	if r, ok := s.replyFrom[to]; ok {
		from = r
	}
	s.mu.Unlock()

	if len(b) > maxDatagram(to.name) {
		return os.NewSyscallError(call, syscall.EMSGSIZE)
	}
	if !s.host.deliver(datagram{b: bytes.Clone(b), from: from, to: to}) && s.dialled {
		s.mu.Lock()
		s.err = syscall.ECONNREFUSED
		s.signalLocked()
		s.mu.Unlock()
	}
	return nil
}

// enqueue queues d for a read, and reports whether s took it. A closed
// socket takes nothing, and a dialled one takes only what its peer sends.
func (s *socket) enqueue(d datagram) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if isClosed(s.done) || s.dialled && d.from != s.remote {
		return false
	}
	if s.local.everyName() {
		s.replyFrom[d.from] = d.to
	}
	s.queue = append(s.queue, d)
	s.signalLocked()
	return true
}

// signalLocked leaves a token in ready when a read has something to
// return.
func (s *socket) signalLocked() {
	if len(s.queue) == 0 && s.err == nil {
		return
	}
	select {
	case s.ready <- struct{}{}:
	default:
	}
}

// Close closes the socket. A read waiting on it returns at once. Closing
// it again returns an error wrapping net.ErrClosed, as closing a real
// socket twice does.
func (s *socket) Close() error {
	closed := false
	s.once.Do(func() {
		closed = true
		close(s.done)
		s.host.removeSocket(s)
	})
	if !closed {
		return s.opErr("close", net.ErrClosed)
	}
	return nil
}

// LocalAddr returns the address the socket holds: its host as written and
// its port.
func (s *socket) LocalAddr() net.Addr {
	return s.local.udpAddr()
}

// SetDeadline sets the read and the write deadlines, as SetReadDeadline
// and SetWriteDeadline do.
func (s *socket) SetDeadline(t time.Time) error {
	if isClosed(s.done) {
		return s.setErr()
	}
	s.rd.set(t)
	s.wd.set(t)
	return nil
}

// SetReadDeadline sets the time after which a read fails with an error
// wrapping os.ErrDeadlineExceeded, including a read already waiting. The
// zero time means no deadline. The deadline runs on the clock of the
// testing/synctest bubble the socket was made in.
func (s *socket) SetReadDeadline(t time.Time) error {
	if isClosed(s.done) {
		return s.setErr()
	}
	s.rd.set(t)
	return nil
}

// SetWriteDeadline sets the time after which a write fails with an error
// wrapping os.ErrDeadlineExceeded. The zero time means no deadline. The
// deadline runs on the clock of the testing/synctest bubble the socket was
// made in.
func (s *socket) SetWriteDeadline(t time.Time) error {
	if isClosed(s.done) {
		return s.setErr()
	}
	s.wd.set(t)
	return nil
}

// SetReadBuffer succeeds and does nothing. It is there because a
// *net.UDPConn has it, and code such as a QUIC implementation calls it.
func (*socket) SetReadBuffer(int) error { return nil }

// SetWriteBuffer succeeds and does nothing. It is there because a
// *net.UDPConn has it, and code such as a QUIC implementation calls it.
func (*socket) SetWriteBuffer(int) error { return nil }

func (s *socket) opErr(op string, err error) error {
	var remote net.Addr
	if s.dialled {
		remote = s.remote.udpAddr()
	}
	return &net.OpError{Op: op, Net: s.network, Source: s.LocalAddr(), Addr: remote, Err: err}
}

func (s *socket) setErr() error {
	return &net.OpError{Op: "set", Net: s.network, Addr: s.LocalAddr(), Err: net.ErrClosed}
}

// packetConn is the net.PacketConn that Host.ListenPacket returns.
type packetConn struct{ *socket }

// ReadFrom waits for a datagram sent to an address the socket holds and
// copies it into b, truncating it if b is short, as a real socket does.
// It returns the address the datagram is from.
func (c packetConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, from, err := c.recv(b)
	if err != nil {
		return 0, nil, c.opErr("read", err)
	}
	return n, from.udpAddr(), nil
}

// WriteTo sends b to a, as one datagram, which is dropped if no socket
// holds a. a is any net.Addr whose String is a host and port, such as the
// address ReadFrom returns.
func (c packetConn) WriteTo(b []byte, a net.Addr) (int, error) {
	if a == nil {
		return 0, c.opErr("write", syscall.EINVAL)
	}
	to, err := parseAddress(a.String())
	if err == nil {
		err = c.send(b, to, "sendto")
	}
	if err != nil {
		return 0, &net.OpError{Op: "write", Net: c.network, Source: c.LocalAddr(), Addr: a, Err: err}
	}
	return len(b), nil
}

// udpConn is the net.Conn that Host.Dial returns over UDP.
type udpConn struct{ *socket }

// Read waits for a datagram from the address the socket dialled and
// copies it into b, truncating it if b is short, as a real socket does.
func (c udpConn) Read(b []byte) (int, error) {
	n, _, err := c.recv(b)
	if err != nil {
		return 0, c.opErr("read", err)
	}
	return n, nil
}

// Write sends b to the address the socket dialled, as one datagram.
func (c udpConn) Write(b []byte) (int, error) {
	if err := c.send(b, c.remote, "write"); err != nil {
		return 0, c.opErr("write", err)
	}
	return len(b), nil
}

// RemoteAddr returns the address the socket dialled, as written.
func (c udpConn) RemoteAddr() net.Addr {
	return c.remote.udpAddr()
}

// deadline is a read or a write deadline: a channel closed once it has
// passed, on the clock of the bubble it was set in. It works like
// net.Pipe's.
type deadline struct {
	mu     sync.Mutex
	timer  *time.Timer
	cancel chan struct{}
}

// set sets the deadline to t, or clears it when t is zero.
func (d *deadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil && !d.timer.Stop() {
		<-d.cancel // the timer fired: wait for it to close cancel
	}
	d.timer = nil
	if isClosed(d.cancel) {
		d.cancel = make(chan struct{})
	}
	if t.IsZero() {
		return
	}
	dur := time.Until(t)
	if dur <= 0 {
		close(d.cancel)
		return
	}
	cancel := d.cancel
	d.timer = time.AfterFunc(dur, func() { close(cancel) })
}

// wait returns a channel closed once the deadline passes.
func (d *deadline) wait() chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cancel
}
