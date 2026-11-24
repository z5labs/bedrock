// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest

import (
	"bytes"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// pipeBuffer is how many bytes each direction of a TCP connection holds
// before a Write waits for the peer to read, about what a loopback
// socket's buffers hold.
const pipeBuffer = 256 << 10

// newPipe returns the two ends of an in-memory TCP connection. Each
// direction holds up to pipeBuffer bytes, as a real socket's buffers do: a
// Write returns once its bytes are buffered, not once the peer reads them,
// so a write the peer never reads, such as a TLS close_notify alert,
// doesn't hold its writer. It starts no goroutines of its own; a deadline
// is a time.AfterFunc. Every call waits on channels, so a connection made
// inside a testing/synctest bubble blocks durably in it.
func newPipe() (*pipeEnd, *pipeEnd) {
	ab, ba := newStream(), newStream()
	return newPipeEnd(ba, ab), newPipeEnd(ab, ba)
}

// stream is one direction of a connection: the bytes one end has written
// and the other end has yet to read.
type stream struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	wclosed bool          // the writing end is closed: reads drain buf, then io.EOF
	rclosed bool          // the reading end is closed: writes fail
	changed chan struct{} // closed, and replaced, whenever any of the above changes
}

func newStream() *stream {
	return &stream{changed: make(chan struct{})}
}

// notifyLocked wakes every call waiting on s.
func (s *stream) notifyLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// pipeEnd is one end of a connection made by newPipe. local and remote are
// the addresses it reports: DialTCP sets the listener's address as the
// server's local address and the client's remote one.
type pipeEnd struct {
	in, out       *stream
	local, remote net.Addr

	rd, wd deadline
	once   sync.Once
}

func newPipeEnd(in, out *stream) *pipeEnd {
	return &pipeEnd{
		in:     in,
		out:    out,
		local:  pipeAddr{},
		remote: pipeAddr{},
		rd:     deadline{cancel: make(chan struct{})},
		wd:     deadline{cancel: make(chan struct{})},
	}
}

// Read reads what the peer has written. Once the peer has closed, it
// returns what is left and then io.EOF. After this end closes, it returns
// io.ErrClosedPipe, and once the read deadline passes,
// os.ErrDeadlineExceeded.
func (p *pipeEnd) Read(b []byte) (int, error) {
	for {
		expired := p.rd.wait()
		s := p.in
		s.mu.Lock()
		switch {
		case s.rclosed:
			s.mu.Unlock()
			return 0, io.ErrClosedPipe
		case isClosed(expired):
			s.mu.Unlock()
			return 0, os.ErrDeadlineExceeded
		case s.buf.Len() > 0 || len(b) == 0:
			n, _ := s.buf.Read(b)
			if n > 0 {
				s.notifyLocked()
			}
			s.mu.Unlock()
			return n, nil
		case s.wclosed:
			s.mu.Unlock()
			return 0, io.EOF
		}
		changed := s.changed
		s.mu.Unlock()

		select {
		case <-changed:
		case <-expired:
		}
	}
}

// Write buffers b for the peer to read, and waits only while the buffer is
// full. After either end closes, it returns io.ErrClosedPipe, and once the
// write deadline passes, os.ErrDeadlineExceeded, with the number of bytes
// buffered before then.
func (p *pipeEnd) Write(b []byte) (int, error) {
	var n int
	for {
		expired := p.wd.wait()
		s := p.out
		s.mu.Lock()
		switch {
		case s.wclosed, s.rclosed:
			s.mu.Unlock()
			return n, io.ErrClosedPipe
		case isClosed(expired):
			s.mu.Unlock()
			return n, os.ErrDeadlineExceeded
		case len(b) == 0:
			s.mu.Unlock()
			return n, nil
		}
		if space := pipeBuffer - s.buf.Len(); space > 0 {
			k := min(space, len(b))
			s.buf.Write(b[:k])
			b = b[k:]
			n += k
			s.notifyLocked()
			s.mu.Unlock()
			// Once every byte is buffered, the Write has succeeded, even
			// if the peer reads them and closes before it returns.
			if len(b) == 0 {
				return n, nil
			}
			continue
		}
		changed := s.changed
		s.mu.Unlock()

		select {
		case <-changed:
		case <-expired:
		}
	}
}

// Close closes this end. Its own calls, and the peer's writes, fail with
// io.ErrClosedPipe, and the peer reads what this end wrote and then
// io.EOF. Closing it again does nothing.
func (p *pipeEnd) Close() error {
	p.once.Do(func() {
		// Writes fail first, so that nothing a failed Read wakes, such as
		// an http.Server's handler, can write after the close.
		p.out.mu.Lock()
		p.out.wclosed = true
		p.out.notifyLocked()
		p.out.mu.Unlock()

		p.in.mu.Lock()
		p.in.rclosed = true
		p.in.buf = bytes.Buffer{}
		p.in.notifyLocked()
		p.in.mu.Unlock()
	})
	return nil
}

func (p *pipeEnd) LocalAddr() net.Addr  { return p.local }
func (p *pipeEnd) RemoteAddr() net.Addr { return p.remote }

// SetDeadline sets the read and the write deadlines, as SetReadDeadline
// and SetWriteDeadline do.
func (p *pipeEnd) SetDeadline(t time.Time) error {
	if err := p.SetReadDeadline(t); err != nil {
		return err
	}
	return p.SetWriteDeadline(t)
}

// SetReadDeadline sets the read deadline, as on a real socket, including
// for a Read already waiting. The deadline runs on the clock of the
// testing/synctest bubble it is set in. It fails with io.ErrClosedPipe
// once this end is closed.
func (p *pipeEnd) SetReadDeadline(t time.Time) error {
	if p.closed() {
		return io.ErrClosedPipe
	}
	p.rd.set(t)
	return nil
}

// SetWriteDeadline sets the write deadline, as on a real socket, including
// for a Write already waiting. The deadline runs on the clock of the
// testing/synctest bubble it is set in. It fails with io.ErrClosedPipe
// once this end is closed.
func (p *pipeEnd) SetWriteDeadline(t time.Time) error {
	if p.closed() {
		return io.ErrClosedPipe
	}
	p.wd.set(t)
	return nil
}

func (p *pipeEnd) closed() bool {
	p.out.mu.Lock()
	defer p.out.mu.Unlock()
	return p.out.wclosed
}

// pipeAddr is the address an end reports when DialTCP sets none: the
// client's local address and the server's remote one.
type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }
