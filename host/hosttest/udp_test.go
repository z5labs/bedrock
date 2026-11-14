// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host/hosttest"
)

// Every test in this file runs on the in-memory network, inside a
// testing/synctest bubble, and opens no real socket.

func listenPacket(t *testing.T, h *hosttest.Host, address string) net.PacketConn {
	t.Helper()
	pc, err := h.ListenPacket("udp", address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pc.Close() })
	return pc
}

func dialUDP(t *testing.T, h *hosttest.Host, address string) net.Conn {
	t.Helper()
	c, err := h.Dial(t.Context(), "udp", address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// readFrom reads one datagram, which must already have arrived: inside a
// bubble, a read that would wait for one fails the test with a deadlock.
func readFrom(t *testing.T, pc net.PacketConn) (string, net.Addr) {
	t.Helper()
	b := make([]byte, 1<<16)
	n, from, err := pc.ReadFrom(b)
	require.NoError(t, err)
	return string(b[:n]), from
}

func readConn(t *testing.T, c net.Conn) string {
	t.Helper()
	b := make([]byte, 1<<16)
	n, err := c.Read(b)
	require.NoError(t, err)
	return string(b[:n])
}

func writeTo(t *testing.T, pc net.PacketConn, msg string, to net.Addr) {
	t.Helper()
	n, err := pc.WriteTo([]byte(msg), to)
	require.NoError(t, err)
	require.Equal(t, len(msg), n)
}

// requireNothingToRead shows that nothing waits to be read on pc: a read
// with a deadline a second away fails when the second passes.
func requireNothingToRead(t *testing.T, pc net.PacketConn) {
	t.Helper()
	require.NoError(t, pc.SetReadDeadline(time.Now().Add(time.Second)))
	_, _, err := pc.ReadFrom(make([]byte, 64))
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	require.NoError(t, pc.SetReadDeadline(time.Time{}))
}

// asNetOpError checks that err is a *net.OpError for op, and returns it.
func asNetOpError(t *testing.T, err error, op string) *net.OpError {
	t.Helper()
	requireNetOpError(t, err, op)
	opErr, _ := errors.AsType[*net.OpError](err)
	return opErr
}

// requireNetOpError checks that err is a *net.OpError for op.
func requireNetOpError(t *testing.T, err error, op string) {
	t.Helper()
	opErr, ok := errors.AsType[*net.OpError](err)
	require.True(t, ok, "%T is not a *net.OpError", err)
	require.Equal(t, op, opErr.Op)
}

func TestHost_ListenPacket_EADDRINUSE(t *testing.T) {
	testCases := []struct {
		name          string
		first, second string
	}{
		{name: "the same name and port twice", first: "api:53", second: "api:53"},
		{name: "every name after a name", first: "api:53", second: ":53"},
		{name: "0.0.0.0 after a name", first: "api:53", second: "0.0.0.0:53"},
		{name: ":: after a name", first: "api:53", second: "[::]:53"},
		{name: "a name after every name", first: ":53", second: "api:53"},
		{name: "a name after 0.0.0.0", first: "0.0.0.0:53", second: "api:53"},
		{name: "every name after every name", first: "0.0.0.0:53", second: ":53"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := hosttest.New(t)
				listenPacket(t, h, tc.first)

				pc, err := h.ListenPacket("udp", tc.second)
				require.Nil(t, pc)
				require.ErrorIs(t, err, syscall.EADDRINUSE)
				opErr := asNetOpError(t, err, "listen")
				require.Equal(t, tc.second, opErr.Addr.String())
				require.Equal(t, "udp", opErr.Addr.Network())
			})
		})
	}
}

func TestHost_ListenPacket_binds(t *testing.T) {
	t.Run("two names on one port", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			api := listenPacket(t, h, "api.internal:53")
			admin := listenPacket(t, h, "admin.internal:53")
			client := listenPacket(t, h, "client:9")

			writeTo(t, client, "api", api.LocalAddr())
			writeTo(t, client, "admin", admin.LocalAddr())
			got, _ := readFrom(t, api)
			require.Equal(t, "api", got)
			got, _ = readFrom(t, admin)
			require.Equal(t, "admin", got)
		})
	})

	t.Run("names are compared as written and never resolved", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			ip := listenPacket(t, h, "127.0.0.1:53")
			client := listenPacket(t, h, "client:9")

			writeTo(t, client, "lost", hostAddr("localhost:53"))
			requireNothingToRead(t, ip)
		})
	})

	t.Run("port 0 picks distinct ports from IANA's dynamic range", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			a := listenPacket(t, h, "api:0")
			b := listenPacket(t, h, "api:")

			pa, pb := portOf(t, a.LocalAddr()), portOf(t, b.LocalAddr())
			require.NotEqual(t, pa, pb)
			for _, p := range []int{pa, pb} {
				require.GreaterOrEqual(t, p, 49152)
				require.LessOrEqual(t, p, 65535)
			}
			require.Equal(t, "api:"+strconv.Itoa(pa), a.LocalAddr().String())
			require.Equal(t, "udp", a.LocalAddr().Network())
		})
	})

	t.Run("reports every name as written", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			for _, host := range []string{"", "0.0.0.0", "::"} {
				pc, err := h.ListenPacket("udp", net.JoinHostPort(host, "0"))
				require.NoError(t, err)
				require.Equal(t, net.JoinHostPort(host, strconv.Itoa(portOf(t, pc.LocalAddr()))), pc.LocalAddr().String())
				require.NoError(t, pc.Close())
			}
		})
	})

	t.Run("fails with EADDRINUSE when every port is held", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			var pcs []net.PacketConn
			for {
				pc, err := h.ListenPacket("udp", ":0")
				if err != nil {
					require.ErrorIs(t, err, syscall.EADDRINUSE)
					requireNetOpError(t, err, "listen")
					break
				}
				pcs = append(pcs, pc)
			}
			require.Len(t, pcs, 65535-49152+1)

			_, err := h.Dial(t.Context(), "udp", "api:53")
			require.ErrorIs(t, err, syscall.EADDRINUSE, "a dialled socket needs a port too")
			opErr := asNetOpError(t, err, "dial")
			require.Equal(t, "api:53", opErr.Addr.String())

			for _, pc := range pcs {
				require.NoError(t, pc.Close())
			}
		})
	})

	t.Run("udp4 and udp6 share udp's ports", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc, err := h.ListenPacket("udp4", "api:53")
			require.NoError(t, err)
			defer func() { require.NoError(t, pc.Close()) }()

			for _, network := range []string{"udp", "udp6"} {
				_, err := h.ListenPacket(network, "api:53")
				require.ErrorIs(t, err, syscall.EADDRINUSE, network)
			}
		})
	})

	t.Run("TCP and UDP bind the same port either way round", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			https := listen(t, h, ":443")
			defer func() { require.NoError(t, https.Close()) }()
			listenPacket(t, h, ":443")

			listenPacket(t, h, ":853")
			require.NoError(t, listen(t, h, ":853").Close())
		})
	})

	t.Run("a port can be bound again after its socket closes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			for _, address := range []string{"api:53", ":53", "api:53"} {
				pc, err := h.ListenPacket("udp", address)
				require.NoError(t, err)
				require.NoError(t, pc.Close())
			}
		})
	})
}

func TestHost_udp_malformed_addresses(t *testing.T) {
	testCases := []struct {
		name    string
		network string
		address string
	}{
		{name: "no port", network: "udp", address: "api"},
		{name: "a service name for a port", network: "udp", address: "api:domain"},
		{name: "a port out of range", network: "udp", address: "api:65536"},
		{name: "a network that is not UDP", network: "unixgram", address: "api:53"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := hosttest.New(t)

				pc, err := h.ListenPacket(tc.network, tc.address)
				require.Nil(t, pc)
				opErr := asNetOpError(t, err, "listen")
				require.Equal(t, tc.network, opErr.Net)

				c, err := h.Dial(t.Context(), tc.network, tc.address)
				require.Nil(t, c)
				requireNetOpError(t, err, "dial")
			})
		})
	}

	t.Run("ListenPacket does not listen over TCP, nor Listen over UDP", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)

			_, err := h.ListenPacket("tcp", "api:53")
			var unknown net.UnknownNetworkError
			require.ErrorAs(t, err, &unknown)
			requireNetOpError(t, err, "listen")

			_, err = h.Listen("udp", "api:53")
			require.ErrorAs(t, err, &unknown)
			requireNetOpError(t, err, "listen")
		})
	})
}

func TestHost_udp_datagrams(t *testing.T) {
	t.Run("arrive whole and in order, from the sender's address", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "dns:53")
			client := dialUDP(t, h, "dns:53")

			for _, msg := range []string{"a", "", "ccc"} {
				_, err := client.Write([]byte(msg))
				require.NoError(t, err)
			}
			for _, want := range []string{"a", "", "ccc"} {
				got, from := readFrom(t, server)
				require.Equal(t, want, got)
				require.Equal(t, client.LocalAddr(), from)
			}
		})
	})

	t.Run("a socket on every name takes datagrams to any name on its port", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, ":53")
			client := listenPacket(t, h, "client:9")

			for _, name := range []string{"api", "dns", "127.0.0.1"} {
				writeTo(t, client, name, hostAddr(net.JoinHostPort(name, "53")))
				got, _ := readFrom(t, server)
				require.Equal(t, name, got)
			}
		})
	})

	t.Run("are dropped when no socket holds the address", func(t *testing.T) {
		testCases := []struct {
			name, bind, to string
		}{
			{name: "nothing is bound", to: "api:53"},
			{name: "another port", bind: "api:54", to: "api:53"},
			{name: "another name", bind: "api:53", to: "dns:53"},
			{name: "every name on another port", bind: ":54", to: "api:53"},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					h := hosttest.New(t)
					client := listenPacket(t, h, "client:9")
					writeTo(t, client, "lost", hostAddr(tc.to))
					if tc.bind != "" {
						requireNothingToRead(t, listenPacket(t, h, tc.bind))
					}
					requireNothingToRead(t, client)
				})
			})
		}
	})

	t.Run("are dropped once the socket that held the address closes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "dns:53")
			require.NoError(t, server.Close())
			client := listenPacket(t, h, "client:9")
			writeTo(t, client, "lost", server.LocalAddr())

			again := listenPacket(t, h, "dns:53")
			requireNothingToRead(t, again)
		})
	})

	t.Run("are copied when they are written", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "dns:53")
			client := dialUDP(t, h, "dns:53")

			b := []byte("before")
			_, err := client.Write(b)
			require.NoError(t, err)
			copy(b, "after!")
			got, _ := readFrom(t, server)
			require.Equal(t, "before", got)
		})
	})

	t.Run("a read into a short buffer truncates the datagram", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "dns:53")
			client := dialUDP(t, h, "dns:53")
			for _, msg := range []string{"0123456789", "next", "reply"} {
				_, err := client.Write([]byte(msg))
				require.NoError(t, err)
			}

			b := make([]byte, 4)
			n, _, err := server.ReadFrom(b)
			require.NoError(t, err)
			require.Equal(t, "0123", string(b[:n]))
			got, from := readFrom(t, server)
			require.Equal(t, "next", got, "the rest of the first datagram is discarded")
			readFrom(t, server)

			writeTo(t, server, "0123456789", from)
			n, err = client.Read(b)
			require.NoError(t, err)
			require.Equal(t, "0123", string(b[:n]))
		})
	})

	t.Run("reach every reader waiting on one socket", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "dns:53")
			client := dialUDP(t, h, "dns:53")

			got := make(chan string, 2)
			var wg sync.WaitGroup
			for range 2 {
				wg.Go(func() {
					b := make([]byte, 64)
					n, _, err := server.ReadFrom(b)
					if err == nil {
						got <- string(b[:n])
					}
				})
			}
			synctest.Wait()
			for _, msg := range []string{"one", "two"} {
				_, err := client.Write([]byte(msg))
				require.NoError(t, err)
			}
			wg.Wait()
			close(got)
			var all []string
			for s := range got {
				all = append(all, s)
			}
			require.ElementsMatch(t, []string{"one", "two"}, all)
		})
	})

	t.Run("arrive whole from many writers at once", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, ":53")

			const writers = 8
			var wg sync.WaitGroup
			for i := range writers {
				wg.Go(func() {
					c, err := h.Dial(t.Context(), "udp", "dns:53")
					if err != nil {
						t.Error(err)
						return
					}
					defer func() { _ = c.Close() }()
					if _, err := c.Write([]byte(strconv.Itoa(i))); err != nil {
						t.Error(err)
					}
				})
			}
			wg.Wait()
			var all []string
			for range writers {
				got, _ := readFrom(t, server)
				all = append(all, got)
			}
			require.ElementsMatch(t, []string{"0", "1", "2", "3", "4", "5", "6", "7"}, all)
		})
	})
}

func TestHost_udp_EMSGSIZE(t *testing.T) {
	testCases := []struct {
		name    string
		host    string
		largest int
	}{
		{name: "IPv4's limit to a name", host: "dns", largest: 65507},
		{name: "IPv4's limit to an IPv4 address", host: "127.0.0.1", largest: 65507},
		{name: "IPv4's limit to an IPv4-mapped IPv6 address", host: "::ffff:127.0.0.1", largest: 65507},
		{name: "IPv6's limit to an IPv6 address", host: "::1", largest: 65527},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := hosttest.New(t)
				address := net.JoinHostPort(tc.host, "53")
				server := listenPacket(t, h, address)
				client := dialUDP(t, h, address)
				largest := bytes.Repeat([]byte{'x'}, tc.largest)

				_, err := client.Write(largest)
				require.NoError(t, err)
				got, from := readFrom(t, server)
				require.Len(t, got, tc.largest)

				_, err = client.Write(append(largest, 'x'))
				require.ErrorIs(t, err, syscall.EMSGSIZE)
				requireSyscall(t, err, "write")
				opErr := asNetOpError(t, err, "write")
				require.Equal(t, client.LocalAddr(), opErr.Source)
				require.Equal(t, client.RemoteAddr(), opErr.Addr)

				other := listenPacket(t, h, net.JoinHostPort(tc.host, "0"))
				_, err = other.WriteTo(append(largest, 'x'), server.LocalAddr())
				require.ErrorIs(t, err, syscall.EMSGSIZE)
				requireSyscall(t, err, "sendto")
				opErr = asNetOpError(t, err, "write")
				require.Equal(t, server.LocalAddr(), opErr.Addr)

				writeTo(t, server, string(largest), from)
				require.Len(t, readConn(t, client), tc.largest)
			})
		})
	}
}

func requireSyscall(t *testing.T, err error, call string) {
	t.Helper()
	sysErr, ok := errors.AsType[*os.SyscallError](err)
	require.True(t, ok, "%v wraps no *os.SyscallError", err)
	require.Equal(t, call, sysErr.Syscall)
}

func TestHost_udp_replies(t *testing.T) {
	t.Run("a socket on every name replies from the address its peer sent to", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, ":53")

			for _, name := range []string{"api", "dns", "", "0.0.0.0"} {
				address := net.JoinHostPort(name, "53")
				client := dialUDP(t, h, address)
				_, err := client.Write([]byte("ping"))
				require.NoError(t, err)
				_, from := readFrom(t, server)

				writeTo(t, server, "pong", from)
				require.Equal(t, "pong", readConn(t, client), "the reply to %s came from %s", from, address)
			}
		})
	})

	t.Run("a socket on every name sends to a peer it has not heard from from its own address", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "0.0.0.0:53")
			peer := listenPacket(t, h, "peer:9")

			writeTo(t, server, "hello", peer.LocalAddr())
			_, from := readFrom(t, peer)
			require.Equal(t, "0.0.0.0:53", from.String())
		})
	})

	t.Run("a socket on a name sends from that name", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "dns:53")
			peer := listenPacket(t, h, "peer:9")

			writeTo(t, server, "hello", peer.LocalAddr())
			_, from := readFrom(t, peer)
			require.Equal(t, server.LocalAddr(), from)
		})
	})

	t.Run("WriteTo takes any address whose String is a host and port", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "127.0.0.1:53")
			peer := listenPacket(t, h, "peer:9")

			writeTo(t, peer, "hello", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53})
			got, _ := readFrom(t, server)
			require.Equal(t, "hello", got)
		})
	})

	t.Run("WriteTo fails for no address or a malformed one", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			peer := listenPacket(t, h, "peer:9")

			_, err := peer.WriteTo([]byte("x"), nil)
			require.ErrorIs(t, err, syscall.EINVAL)
			requireNetOpError(t, err, "write")

			_, err = peer.WriteTo([]byte("x"), hostAddr("no-port"))
			opErr := asNetOpError(t, err, "write")
			require.Equal(t, "no-port", opErr.Addr.String())
		})
	})
}

func TestHost_Dial_udp(t *testing.T) {
	t.Run("binds the name it dials and a port from IANA's dynamic range", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			c := dialUDP(t, h, "dns:53")

			require.Equal(t, "dns:53", c.RemoteAddr().String())
			require.Equal(t, "udp", c.RemoteAddr().Network())
			name, _, err := net.SplitHostPort(c.LocalAddr().String())
			require.NoError(t, err)
			require.Equal(t, "dns", name)
			require.Equal(t, "udp", c.LocalAddr().Network())
			port := portOf(t, c.LocalAddr())
			require.GreaterOrEqual(t, port, 49152)
			require.LessOrEqual(t, port, 65535)

			_, err = h.ListenPacket("udp", c.LocalAddr().String())
			require.ErrorIs(t, err, syscall.EADDRINUSE, "the dialled socket holds its port")
		})
	})

	t.Run("succeeds whether or not a socket holds the address", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			for _, network := range []string{"udp", "udp4", "udp6"} {
				c, err := h.Dial(t.Context(), network, "dns:53")
				require.NoError(t, err, network)
				require.NoError(t, c.Close())
			}
		})
	})

	t.Run("takes datagrams only from the address it dialled", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			dialled := listenPacket(t, h, "dns:53")
			other := listenPacket(t, h, "dns:54")
			client := dialUDP(t, h, "dns:53")
			_, err := client.Write([]byte("ping"))
			require.NoError(t, err)
			_, from := readFrom(t, dialled)

			writeTo(t, other, "other", from)
			writeTo(t, dialled, "dialled", from)
			require.Equal(t, "dialled", readConn(t, client))

			require.NoError(t, client.SetReadDeadline(time.Now().Add(time.Second)))
			_, err = client.Read(make([]byte, 64))
			require.ErrorIs(t, err, os.ErrDeadlineExceeded, "the other datagram was dropped")
		})
	})

	t.Run("returns ctx's error when ctx is already done", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			c, err := h.Dial(ctx, "udp", "dns:53")
			require.Nil(t, c)
			require.ErrorIs(t, err, context.Canceled)
			opErr := asNetOpError(t, err, "dial")
			require.Equal(t, "udp", opErr.Addr.Network())
		})
	})
}

func TestHost_udp_ECONNREFUSED(t *testing.T) {
	t.Run("on the read after a write to an address no socket holds", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			client := dialUDP(t, h, "dns:53")
			_, err := client.Write([]byte("anyone?"))
			require.NoError(t, err)

			_, err = client.Read(make([]byte, 64))
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
			requireSyscall(t, err, "read")
			opErr := asNetOpError(t, err, "read")
			require.Equal(t, client.LocalAddr(), opErr.Source)
			require.Equal(t, client.RemoteAddr(), opErr.Addr)

			require.NoError(t, client.SetReadDeadline(time.Now().Add(time.Second)))
			_, err = client.Read(make([]byte, 64))
			require.ErrorIs(t, err, os.ErrDeadlineExceeded, "the refusal is reported once")
		})
	})

	t.Run("on the write after a write to an address no socket holds", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			client := dialUDP(t, h, "dns:53")
			_, err := client.Write([]byte("anyone?"))
			require.NoError(t, err)

			_, err = client.Write([]byte("anyone?"))
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
			requireSyscall(t, err, "write")
			requireNetOpError(t, err, "write")

			_, err = client.Write([]byte("anyone?"))
			require.NoError(t, err, "the refusal is reported once")
		})
	})

	t.Run("after the socket it dialled closes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "dns:53")
			client := dialUDP(t, h, "dns:53")
			_, err := client.Write([]byte("one"))
			require.NoError(t, err)
			readFrom(t, server)
			require.NoError(t, server.Close())

			_, err = client.Write([]byte("two"))
			require.NoError(t, err)
			_, err = client.Read(make([]byte, 64))
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
		})
	})

	t.Run("wakes a read already waiting", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			client := dialUDP(t, h, "dns:53")

			readErr := make(chan error, 1)
			go func() {
				_, err := client.Read(make([]byte, 64))
				readErr <- err
			}()
			synctest.Wait()
			require.Empty(t, readErr)

			_, err := client.Write([]byte("anyone?"))
			require.NoError(t, err)
			require.ErrorIs(t, <-readErr, syscall.ECONNREFUSED)
		})
	})

	t.Run("never for a socket from ListenPacket", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc := listenPacket(t, h, "client:9")
			for range 2 {
				writeTo(t, pc, "anyone?", hostAddr("dns:53"))
			}
			requireNothingToRead(t, pc)
		})
	})
}

func TestHost_udp_deadlines(t *testing.T) {
	t.Run("a read fails when its deadline passes, on the bubble's clock", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc := listenPacket(t, h, "dns:53")

			start := time.Now()
			require.NoError(t, pc.SetReadDeadline(start.Add(time.Second)))
			_, _, err := pc.ReadFrom(make([]byte, 64))
			require.ErrorIs(t, err, os.ErrDeadlineExceeded)
			netErr, ok := errors.AsType[net.Error](err)
			require.True(t, ok)
			require.True(t, netErr.Timeout())
			requireNetOpError(t, err, "read")
			require.Equal(t, time.Second, time.Since(start))
		})
	})

	t.Run("a read already waiting fails when a deadline set later passes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			c := dialUDP(t, h, "dns:53")

			readErr := make(chan error, 1)
			go func() {
				_, err := c.Read(make([]byte, 64))
				readErr <- err
			}()
			synctest.Wait()
			start := time.Now()
			require.NoError(t, c.SetDeadline(start.Add(time.Second)))
			require.ErrorIs(t, <-readErr, os.ErrDeadlineExceeded)
			require.Equal(t, time.Second, time.Since(start))
		})
	})

	t.Run("a later deadline replaces one that has not passed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc := listenPacket(t, h, "dns:53")

			start := time.Now()
			require.NoError(t, pc.SetReadDeadline(start.Add(time.Second)))
			require.NoError(t, pc.SetReadDeadline(start.Add(2*time.Second)))
			_, _, err := pc.ReadFrom(make([]byte, 64))
			require.ErrorIs(t, err, os.ErrDeadlineExceeded)
			require.Equal(t, 2*time.Second, time.Since(start))
		})
	})

	t.Run("a deadline in the past fails a read at once, and the zero time clears it", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "dns:53")
			client := dialUDP(t, h, "dns:53")
			_, err := client.Write([]byte("waiting"))
			require.NoError(t, err)

			require.NoError(t, server.SetReadDeadline(time.Now().Add(-time.Second)))
			_, _, err = server.ReadFrom(make([]byte, 64))
			require.ErrorIs(t, err, os.ErrDeadlineExceeded, "even with a datagram waiting")

			require.NoError(t, server.SetReadDeadline(time.Time{}))
			got, _ := readFrom(t, server)
			require.Equal(t, "waiting", got)
		})
	})

	t.Run("a deadline that passed is cleared by the zero time", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "dns:53")
			client := dialUDP(t, h, "dns:53")

			require.NoError(t, server.SetReadDeadline(time.Now().Add(time.Second)))
			time.Sleep(2 * time.Second)
			require.NoError(t, server.SetReadDeadline(time.Time{}))

			_, err := client.Write([]byte("in time"))
			require.NoError(t, err)
			got, _ := readFrom(t, server)
			require.Equal(t, "in time", got)
		})
	})

	t.Run("a write fails once its deadline has passed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			server := listenPacket(t, h, "dns:53")
			client := dialUDP(t, h, "dns:53")

			require.NoError(t, client.SetWriteDeadline(time.Now().Add(time.Second)))
			_, err := client.Write([]byte("in time"))
			require.NoError(t, err)
			time.Sleep(2 * time.Second)
			_, err = client.Write([]byte("late"))
			require.ErrorIs(t, err, os.ErrDeadlineExceeded)
			requireNetOpError(t, err, "write")

			require.NoError(t, server.SetDeadline(time.Now().Add(-time.Second)))
			_, err = server.WriteTo([]byte("late"), client.LocalAddr())
			require.ErrorIs(t, err, os.ErrDeadlineExceeded)
			requireNetOpError(t, err, "write")
		})
	})
}

func TestHost_udp_Close(t *testing.T) {
	t.Run("a read and a write on a closed socket fail with net.ErrClosed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc := listenPacket(t, h, "dns:53")
			c := dialUDP(t, h, "dns:53")
			require.NoError(t, pc.Close())
			require.NoError(t, c.Close())

			_, _, err := pc.ReadFrom(make([]byte, 64))
			require.ErrorIs(t, err, net.ErrClosed)
			requireNetOpError(t, err, "read")
			_, err = pc.WriteTo([]byte("x"), c.RemoteAddr())
			require.ErrorIs(t, err, net.ErrClosed)
			requireNetOpError(t, err, "write")

			_, err = c.Read(make([]byte, 64))
			require.ErrorIs(t, err, net.ErrClosed)
			_, err = c.Write([]byte("x"))
			require.ErrorIs(t, err, net.ErrClosed)
		})
	})

	t.Run("a second Close fails with net.ErrClosed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc := listenPacket(t, h, "dns:53")
			c := dialUDP(t, h, "dns:53")
			for _, closer := range []interface{ Close() error }{pc, c} {
				require.NoError(t, closer.Close())
				err := closer.Close()
				require.ErrorIs(t, err, net.ErrClosed)
				requireNetOpError(t, err, "close")
			}
		})
	})

	t.Run("wakes a read already waiting", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc := listenPacket(t, h, "dns:53")

			readErr := make(chan error, 1)
			go func() {
				_, _, err := pc.ReadFrom(make([]byte, 64))
				readErr <- err
			}()
			synctest.Wait()
			require.Empty(t, readErr)
			require.NoError(t, pc.Close())
			require.ErrorIs(t, <-readErr, net.ErrClosed)
		})
	})

	t.Run("deadlines can't be set on a closed socket", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc := listenPacket(t, h, "dns:53")
			require.NoError(t, pc.Close())

			for _, set := range []func(time.Time) error{pc.SetDeadline, pc.SetReadDeadline, pc.SetWriteDeadline} {
				err := set(time.Now())
				require.ErrorIs(t, err, net.ErrClosed)
				opErr := asNetOpError(t, err, "set")
				require.Equal(t, pc.LocalAddr(), opErr.Addr)
			}
		})
	})
}

func TestHost_udp_SetReadBuffer_and_SetWriteBuffer_succeed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := hosttest.New(t)
		pc := listenPacket(t, h, "dns:53")
		c := dialUDP(t, h, "dns:53")

		for _, s := range []any{pc, c} {
			b, ok := s.(interface {
				SetReadBuffer(bytes int) error
				SetWriteBuffer(bytes int) error
			})
			require.True(t, ok, "%T", s)
			require.NoError(t, b.SetReadBuffer(0))
			require.NoError(t, b.SetWriteBuffer(1<<30))
		}
	})
}

// hostAddr is a net.Addr of a host and port, as a test writes it.
type hostAddr string

func (hostAddr) Network() string  { return "udp" }
func (a hostAddr) String() string { return string(a) }
