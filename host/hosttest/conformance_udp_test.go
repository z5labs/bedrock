// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"bytes"
	"errors"
	"net"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// udpNetwork is the network under test for the UDP scenarios: the Listen,
// ListenPacket and Dial of a host.Host.
type udpNetwork struct {
	listen       listenFunc
	listenPacket func(network, address string) (net.PacketConn, error)
	dial         dialFunc
}

// udpTimeout bounds every wait for a datagram, so a datagram that never
// arrives fails a loopback scenario rather than hanging it. Inside a
// synctest bubble it passes only when every goroutine is blocked.
const udpTimeout = 5 * time.Second

// testConformanceUDP runs the UDP scenarios that the in-memory network
// must get right exactly as a real one does, on the network that run
// makes for each, as testConformance does for TCP.
//
// Every scenario sends to 127.0.0.1 or 127.0.0.2 and takes port 0 for its
// first socket. A socket on every name replies to 127.0.0.1 only: on a
// real network the reply's source address is the one the route to the
// peer picks, which over loopback is 127.0.0.1 whatever the peer sent to.
func testConformanceUDP(t *testing.T, run func(t *testing.T, scenario func(t *testing.T, n udpNetwork))) {
	t.Run("TCP and UDP bind the same port", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			ln, err := n.listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer func() { require.NoError(t, ln.Close()) }()

			pc := udpListen(t, n, net.JoinHostPort("127.0.0.1", strconv.Itoa(portOf(t, ln.Addr()))))
			require.Equal(t, portOf(t, ln.Addr()), portOf(t, pc.LocalAddr()))
		})
	})

	t.Run("two names on one port both receive", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			one := udpListen(t, n, "127.0.0.1:0")
			port := strconv.Itoa(portOf(t, one.LocalAddr()))
			two := udpListen(t, n, net.JoinHostPort("127.0.0.2", port))

			udpWrite(t, udpDial(t, n, net.JoinHostPort("127.0.0.1", port)), "one")
			udpWrite(t, udpDial(t, n, net.JoinHostPort("127.0.0.2", port)), "two")

			got, _ := udpReadFrom(t, one, 64)
			require.Equal(t, "one", got)
			got, _ = udpReadFrom(t, two, 64)
			require.Equal(t, "two", got)
		})
	})

	t.Run("EADDRINUSE for a name on a port every name holds", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			every := udpListen(t, n, ":0")

			_, err := n.listenPacket("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(portOf(t, every.LocalAddr()))))
			requireOpError(t, err, syscall.EADDRINUSE)
		})
	})

	t.Run("EADDRINUSE for every name on a port a name holds", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			one := udpListen(t, n, "127.0.0.1:0")

			_, err := n.listenPacket("udp", ":"+strconv.Itoa(portOf(t, one.LocalAddr())))
			requireOpError(t, err, syscall.EADDRINUSE)
		})
	})

	t.Run("EADDRINUSE for a name and port taken twice", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			one := udpListen(t, n, "127.0.0.1:0")

			_, err := n.listenPacket("udp", one.LocalAddr().String())
			requireOpError(t, err, syscall.EADDRINUSE)
		})
	})

	t.Run("datagrams arrive whole and in order", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			server := udpListen(t, n, "127.0.0.1:0")
			client := udpDial(t, n, server.LocalAddr().String())

			for _, msg := range []string{"a", "bb", "ccc"} {
				udpWrite(t, client, msg)
			}
			for _, want := range []string{"a", "bb", "ccc"} {
				got, from := udpReadFrom(t, server, 64)
				require.Equal(t, want, got)
				require.Equal(t, client.LocalAddr().String(), from.String())
			}
		})
	})

	t.Run("a read into a short buffer truncates the datagram", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			server := udpListen(t, n, "127.0.0.1:0")
			client := udpDial(t, n, server.LocalAddr().String())
			udpWrite(t, client, "0123456789")
			udpWrite(t, client, "next")

			got, _ := udpReadFrom(t, server, 4)
			require.Equal(t, "0123", got, "the rest of the datagram is discarded")
			got, _ = udpReadFrom(t, server, 64)
			require.Equal(t, "next", got)
		})
	})

	t.Run("EMSGSIZE for a datagram too big for UDP", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			server := udpListen(t, n, "127.0.0.1:0")
			client := udpDial(t, n, server.LocalAddr().String())

			largest := bytes.Repeat([]byte{'x'}, 65507)
			_, err := client.Write(largest)
			require.NoError(t, err)
			got, _ := udpReadFrom(t, server, 1<<16)
			require.Len(t, got, len(largest))

			_, err = client.Write(append(largest, 'x'))
			requireOpError(t, err, syscall.EMSGSIZE)
			_, err = server.WriteTo(append(largest, 'x'), client.LocalAddr())
			requireOpError(t, err, syscall.EMSGSIZE)
		})
	})

	t.Run("a socket on every name replies from the address its peer sent to", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			server := udpListen(t, n, ":0")
			client := udpDial(t, n, net.JoinHostPort("127.0.0.1", strconv.Itoa(portOf(t, server.LocalAddr()))))
			udpWrite(t, client, "ping")
			_, from := udpReadFrom(t, server, 64)

			_, err := server.WriteTo([]byte("pong"), from)
			require.NoError(t, err)
			require.Equal(t, "pong", udpRead(t, client, 64), "the dialled socket takes it, so it came from the address dialled")
		})
	})

	t.Run("a dialled socket takes datagrams only from the address it dialled", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			dialled := udpListen(t, n, "127.0.0.1:0")
			other := udpListen(t, n, "127.0.0.1:0")
			client := udpDial(t, n, dialled.LocalAddr().String())
			udpWrite(t, client, "ping")
			_, from := udpReadFrom(t, dialled, 64)

			_, err := other.WriteTo([]byte("other"), from)
			require.NoError(t, err)
			_, err = dialled.WriteTo([]byte("dialled"), from)
			require.NoError(t, err)
			require.Equal(t, "dialled", udpRead(t, client, 64))
		})
	})

	t.Run("ECONNREFUSED after a write to an address no socket holds", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			gone := udpListen(t, n, "127.0.0.1:0")
			address := gone.LocalAddr().String()
			require.NoError(t, gone.Close())

			client := udpDial(t, n, address)
			udpWrite(t, client, "anyone?")
			require.NoError(t, client.SetReadDeadline(time.Now().Add(udpTimeout)))
			_, err := client.Read(make([]byte, 64))
			requireOpError(t, err, syscall.ECONNREFUSED)
		})
	})

	t.Run("a listening socket's write to an address no socket holds succeeds", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			gone := udpListen(t, n, "127.0.0.1:0")
			to := gone.LocalAddr()
			require.NoError(t, gone.Close())

			pc := udpListen(t, n, "127.0.0.1:0")
			for range 2 {
				_, err := pc.WriteTo([]byte("anyone?"), to)
				require.NoError(t, err, "an unconnected socket is never told of the refusal")
			}
		})
	})

	t.Run("a read or write past its deadline fails", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			server := udpListen(t, n, "127.0.0.1:0")
			client := udpDial(t, n, server.LocalAddr().String())

			require.NoError(t, server.SetReadDeadline(time.Now().Add(50*time.Millisecond)))
			_, _, err := server.ReadFrom(make([]byte, 64))
			requireOpError(t, err, os.ErrDeadlineExceeded)
			netErr, ok := errors.AsType[net.Error](err)
			require.True(t, ok)
			require.True(t, netErr.Timeout())

			require.NoError(t, client.SetWriteDeadline(time.Now().Add(-time.Second)))
			_, err = client.Write([]byte("late"))
			requireOpError(t, err, os.ErrDeadlineExceeded)
		})
	})

	t.Run("SetReadBuffer and SetWriteBuffer succeed", func(t *testing.T) {
		run(t, func(t *testing.T, n udpNetwork) {
			server := udpListen(t, n, "127.0.0.1:0")
			client := udpDial(t, n, server.LocalAddr().String())

			for _, c := range []any{server, client} {
				b, ok := c.(interface {
					SetReadBuffer(bytes int) error
					SetWriteBuffer(bytes int) error
				})
				require.True(t, ok, "%T has no SetReadBuffer and SetWriteBuffer", c)
				require.NoError(t, b.SetReadBuffer(1<<20))
				require.NoError(t, b.SetWriteBuffer(1<<20))
			}
		})
	})
}

// udpListen binds a UDP socket on address, closed when the test ends.
func udpListen(t *testing.T, n udpNetwork, address string) net.PacketConn {
	t.Helper()
	pc, err := n.listenPacket("udp", address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pc.Close() })
	return pc
}

// udpDial dials address over UDP, closed when the test ends.
func udpDial(t *testing.T, n udpNetwork, address string) net.Conn {
	t.Helper()
	c, err := n.dial(t.Context(), "udp", address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func udpWrite(t *testing.T, c net.Conn, msg string) {
	t.Helper()
	n, err := c.Write([]byte(msg))
	require.NoError(t, err)
	require.Equal(t, len(msg), n)
}

// udpReadFrom reads one datagram into a buffer of size bytes.
func udpReadFrom(t *testing.T, pc net.PacketConn, size int) (string, net.Addr) {
	t.Helper()
	require.NoError(t, pc.SetReadDeadline(time.Now().Add(udpTimeout)))
	b := make([]byte, size)
	n, from, err := pc.ReadFrom(b)
	require.NoError(t, err)
	return string(b[:n]), from
}

// udpRead reads one datagram into a buffer of size bytes.
func udpRead(t *testing.T, c net.Conn, size int) string {
	t.Helper()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(udpTimeout)))
	b := make([]byte, size)
	n, err := c.Read(b)
	require.NoError(t, err)
	return string(b[:n])
}

// requireOpError checks that err is a *net.OpError and wraps want.
func requireOpError(t *testing.T, err, want error) {
	t.Helper()
	require.ErrorIs(t, err, want)
	_, ok := errors.AsType[*net.OpError](err)
	require.True(t, ok, "%T is not a *net.OpError", err)
}
