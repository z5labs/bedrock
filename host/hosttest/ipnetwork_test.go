// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"context"
	"net"
	"os"
	"strconv"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host"
	"github.com/z5labs/bedrock/host/hosttest"
)

// Every test in this file runs on the in-memory network, inside a
// testing/synctest bubble, and opens no real socket.

// ipNetwork lists the four network methods of host.IPNetwork. The
// assignments below fail to compile if *hosttest.Host, host.IPNetwork or
// hosttest.Seams loses one or changes its signature.
type ipNetwork interface {
	ListenTCP(ctx context.Context, network, address string) (net.Listener, error)
	DialTCP(ctx context.Context, network, address string) (net.Conn, error)
	ListenUDP(ctx context.Context, network, address string) (net.PacketConn, error)
	DialUDP(ctx context.Context, network, address string) (net.Conn, error)
}

var (
	_ ipNetwork = (*hosttest.Host)(nil)
	_ ipNetwork = host.IPNetwork{}
	_ ipNetwork = hosttest.Seams{}
)

// A *hosttest.Host and a host.IPNetwork are each a Seams' Network.
var _ = func(th *hosttest.Host) []hosttest.Seams {
	return []hosttest.Seams{
		{Network: th},
		{Network: host.IPNetwork{}},
	}
}

// requireUnknownNetwork checks that err is the error host.IPNetwork
// returns for a network a method doesn't serve.
func requireUnknownNetwork(t *testing.T, err error, op, network string) {
	t.Helper()
	opErr := asNetOpError(t, err, op)
	require.Equal(t, network, opErr.Net)
	require.Nil(t, opErr.Addr)
	require.Equal(t, net.UnknownNetworkError(network), opErr.Err)
}

// cancelled returns a ctx that is already done.
func cancelled(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

func TestHost_ListenTCP(t *testing.T) {
	t.Run("serves a DialTCP to the name and port it listens on", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			ln, err := h.ListenTCP(t.Context(), "tcp", "api:80")
			require.NoError(t, err)
			serve(t, ln, "api")

			require.Equal(t, "api:80", ln.Addr().String())
			require.Equal(t, "api", dialRead(t, h.DialTCP, "api:80"))
		})
	})

	t.Run("port 0 picks a port from IANA's dynamic range, and Addr reports the name as written", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			ln, err := h.ListenTCP(t.Context(), "tcp", "api:0")
			require.NoError(t, err)
			defer func() { require.NoError(t, ln.Close()) }()

			port := portOf(t, ln.Addr())
			require.GreaterOrEqual(t, port, 49152)
			require.LessOrEqual(t, port, 65535)
			require.Equal(t, "api:"+strconv.Itoa(port), ln.Addr().String())
		})
	})

	t.Run("fails with EADDRINUSE, as a *net.OpError, for a port every name holds", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			every, err := h.ListenTCP(t.Context(), "tcp", ":80")
			require.NoError(t, err)
			defer func() { require.NoError(t, every.Close()) }()

			ln, err := h.ListenTCP(t.Context(), "tcp4", "api:80")
			require.Nil(t, ln)
			require.ErrorIs(t, err, syscall.EADDRINUSE)
			opErr := asNetOpError(t, err, "listen")
			require.Equal(t, "api:80", opErr.Addr.String())
		})
	})

	t.Run("shares one set of ports across its calls", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			ln, err := h.ListenTCP(t.Context(), "tcp", "api:80")
			require.NoError(t, err)
			defer func() { require.NoError(t, ln.Close()) }()

			_, err = h.ListenTCP(t.Context(), "tcp", "api:80")
			require.ErrorIs(t, err, syscall.EADDRINUSE)
		})
	})

	t.Run("compares names as written and never resolves them", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			ln, err := h.ListenTCP(t.Context(), "tcp", "127.0.0.1:80")
			require.NoError(t, err)
			defer func() { require.NoError(t, ln.Close()) }()

			_, err = h.DialTCP(t.Context(), "tcp", "localhost:80")
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
		})
	})

	t.Run("closes a Listening channel for its address", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			listening := h.Listening("api:80")
			require.False(t, isClosed(listening))

			ln, err := h.ListenTCP(t.Context(), "tcp", ":80")
			require.NoError(t, err)
			defer func() { require.NoError(t, ln.Close()) }()
			require.True(t, isClosed(listening))
		})
	})

	t.Run("succeeds with a done ctx", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			ln, err := h.ListenTCP(cancelled(t), "tcp", "api:0")
			require.NoError(t, err)
			require.NoError(t, ln.Close())
		})
	})

	t.Run("fails a malformed address with a *net.OpError", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			ln, err := h.ListenTCP(t.Context(), "tcp", "api:http")
			require.Nil(t, ln)
			opErr := asNetOpError(t, err, "listen")
			require.Equal(t, "tcp", opErr.Net)
		})
	})

	for _, network := range []string{"udp", "udp4", "udp6", "unix", ""} {
		t.Run("refuses network "+strconv.Quote(network), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := hosttest.New(t)
				ln, err := h.ListenTCP(t.Context(), network, "api:80")
				require.Nil(t, ln)
				requireUnknownNetwork(t, err, "listen", network)
			})
		})
	}
}

func TestHost_DialTCP(t *testing.T) {
	t.Run("reaches a listener from ListenTCP, and falls back to the every-name listener", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			serve(t, listen(t, h, "api:80"), "api")
			serve(t, listen(t, h, ":81"), "every")

			for _, network := range []string{"tcp", "tcp4", "tcp6"} {
				dial := func(ctx context.Context, _, address string) (net.Conn, error) {
					return h.DialTCP(ctx, network, address)
				}
				require.Equal(t, "api", dialRead(t, dial, "api:80"), network)
				require.Equal(t, "every", dialRead(t, dial, "admin:81"), network)
			}
		})
	})

	t.Run("fails with ECONNREFUSED, as a *net.OpError, rather than wait for a listener", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			start := time.Now()
			c, err := h.DialTCP(t.Context(), "tcp", "api:80")
			require.Nil(t, c)
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
			opErr := asNetOpError(t, err, "dial")
			require.Equal(t, "api:80", opErr.Addr.String())
			require.Zero(t, time.Since(start))
		})
	})

	t.Run("returns ctx's error when ctx is done before Accept", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			ln := listen(t, h, "api:80")
			defer func() { require.NoError(t, ln.Close()) }()

			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			start := time.Now()
			c, err := h.DialTCP(ctx, "tcp", "api:80")
			require.Nil(t, c)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			requireNetOpError(t, err, "dial")
			require.Equal(t, time.Second, time.Since(start))
		})
	})

	t.Run("returns ctx's error when ctx is already done", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			ln := listen(t, h, "api:80")
			defer func() { require.NoError(t, ln.Close()) }()

			c, err := h.DialTCP(cancelled(t), "tcp", "api:80")
			require.Nil(t, c)
			require.ErrorIs(t, err, context.Canceled)
			requireNetOpError(t, err, "dial")
		})
	})

	for _, network := range []string{"udp", "udp4", "udp6", "unix", ""} {
		t.Run("refuses network "+strconv.Quote(network), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := hosttest.New(t)
				c, err := h.DialTCP(t.Context(), network, "api:80")
				require.Nil(t, c)
				requireUnknownNetwork(t, err, "dial", network)
			})
		})
	}
}

func TestHost_ListenUDP(t *testing.T) {
	t.Run("takes datagrams a DialUDP sends to the name and port it binds", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc, err := h.ListenUDP(t.Context(), "udp", "dns:53")
			require.NoError(t, err)
			defer func() { require.NoError(t, pc.Close()) }()
			require.Equal(t, "dns:53", pc.LocalAddr().String())
			require.Equal(t, "udp", pc.LocalAddr().Network())

			c, err := h.DialUDP(t.Context(), "udp", "dns:53")
			require.NoError(t, err)
			defer func() { require.NoError(t, c.Close()) }()
			_, err = c.Write([]byte("ping"))
			require.NoError(t, err)

			got, from := readFrom(t, pc)
			require.Equal(t, "ping", got)
			require.Equal(t, c.LocalAddr().String(), from.String())
		})
	})

	t.Run("port 0 picks a port from IANA's dynamic range, and LocalAddr reports the name as written", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc, err := h.ListenUDP(t.Context(), "udp", "dns:0")
			require.NoError(t, err)
			defer func() { require.NoError(t, pc.Close()) }()

			port := portOf(t, pc.LocalAddr())
			require.GreaterOrEqual(t, port, 49152)
			require.LessOrEqual(t, port, 65535)
			require.Equal(t, "dns:"+strconv.Itoa(port), pc.LocalAddr().String())
		})
	})

	t.Run("fails with EADDRINUSE, as a *net.OpError, for a port every name holds", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			every, err := h.ListenUDP(t.Context(), "udp", ":53")
			require.NoError(t, err)
			defer func() { require.NoError(t, every.Close()) }()

			pc, err := h.ListenUDP(t.Context(), "udp4", "dns:53")
			require.Nil(t, pc)
			require.ErrorIs(t, err, syscall.EADDRINUSE)
			opErr := asNetOpError(t, err, "listen")
			require.Equal(t, "dns:53", opErr.Addr.String())
			require.Equal(t, "udp", opErr.Addr.Network())
		})
	})

	t.Run("shares one set of ports across its calls, and not with ListenTCP", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc, err := h.ListenUDP(t.Context(), "udp", "dns:53")
			require.NoError(t, err)
			defer func() { require.NoError(t, pc.Close()) }()

			_, err = h.ListenUDP(t.Context(), "udp", "dns:53")
			require.ErrorIs(t, err, syscall.EADDRINUSE)

			ln, err := h.ListenTCP(t.Context(), "tcp", "dns:53")
			require.NoError(t, err)
			require.NoError(t, ln.Close())
		})
	})

	t.Run("compares names as written and never resolves them", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc, err := h.ListenUDP(t.Context(), "udp", "127.0.0.1:53")
			require.NoError(t, err)
			defer func() { require.NoError(t, pc.Close()) }()

			c, err := h.DialUDP(t.Context(), "udp", "localhost:53")
			require.NoError(t, err)
			defer func() { require.NoError(t, c.Close()) }()
			_, err = c.Write([]byte("ping"))
			require.NoError(t, err)
			requireNothingToRead(t, pc)
		})
	})

	t.Run("succeeds with a done ctx", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc, err := h.ListenUDP(cancelled(t), "udp", "dns:0")
			require.NoError(t, err)
			require.NoError(t, pc.Close())
		})
	})

	t.Run("fails a malformed address with a *net.OpError", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			pc, err := h.ListenUDP(t.Context(), "udp", "dns:domain")
			require.Nil(t, pc)
			opErr := asNetOpError(t, err, "listen")
			require.Equal(t, "udp", opErr.Net)
		})
	})

	for _, network := range []string{"tcp", "tcp4", "tcp6", "unixgram", ""} {
		t.Run("refuses network "+strconv.Quote(network), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := hosttest.New(t)
				pc, err := h.ListenUDP(t.Context(), network, "dns:53")
				require.Nil(t, pc)
				requireUnknownNetwork(t, err, "listen", network)
			})
		})
	}
}

func TestHost_DialUDP(t *testing.T) {
	t.Run("binds the name it dials and a port from IANA's dynamic range", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			c, err := h.DialUDP(t.Context(), "udp", "dns:53")
			require.NoError(t, err)
			defer func() { require.NoError(t, c.Close()) }()

			require.Equal(t, "dns:53", c.RemoteAddr().String())
			require.Equal(t, "udp", c.RemoteAddr().Network())
			name, _, err := net.SplitHostPort(c.LocalAddr().String())
			require.NoError(t, err)
			require.Equal(t, "dns", name)
			port := portOf(t, c.LocalAddr())
			require.GreaterOrEqual(t, port, 49152)
			require.LessOrEqual(t, port, 65535)
		})
	})

	t.Run("succeeds at once whether or not a socket holds the address", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			start := time.Now()
			for _, network := range []string{"udp", "udp4", "udp6"} {
				c, err := h.DialUDP(t.Context(), network, "dns:53")
				require.NoError(t, err, network)
				require.NoError(t, c.Close())
			}
			require.Zero(t, time.Since(start))
		})
	})

	t.Run("takes datagrams only from the address it dialled", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			dialled := listenPacket(t, h, "dns:53")
			other := listenPacket(t, h, "dns:54")
			c, err := h.DialUDP(t.Context(), "udp", "dns:53")
			require.NoError(t, err)
			defer func() { require.NoError(t, c.Close()) }()
			_, err = c.Write([]byte("ping"))
			require.NoError(t, err)
			_, from := readFrom(t, dialled)

			writeTo(t, other, "other", from)
			writeTo(t, dialled, "dialled", from)
			require.Equal(t, "dialled", readConn(t, c))

			require.NoError(t, c.SetReadDeadline(time.Now().Add(time.Second)))
			_, err = c.Read(make([]byte, 64))
			require.ErrorIs(t, err, os.ErrDeadlineExceeded, "the other datagram was dropped")
		})
	})

	t.Run("fails with ECONNREFUSED on the read after a write to an address no socket holds", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			c, err := h.DialUDP(t.Context(), "udp", "dns:53")
			require.NoError(t, err)
			defer func() { require.NoError(t, c.Close()) }()
			_, err = c.Write([]byte("anyone?"))
			require.NoError(t, err)

			_, err = c.Read(make([]byte, 64))
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
			requireNetOpError(t, err, "read")
		})
	})

	t.Run("returns ctx's error when ctx is already done", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			c, err := h.DialUDP(cancelled(t), "udp", "dns:53")
			require.Nil(t, c)
			require.ErrorIs(t, err, context.Canceled)
			opErr := asNetOpError(t, err, "dial")
			require.Equal(t, "udp", opErr.Addr.Network())
		})
	})

	for _, network := range []string{"tcp", "tcp4", "tcp6", "unixgram", ""} {
		t.Run("refuses network "+strconv.Quote(network), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := hosttest.New(t)
				c, err := h.DialUDP(t.Context(), network, "dns:53")
				require.Nil(t, c)
				requireUnknownNetwork(t, err, "dial", network)
			})
		})
	}
}

// TestHost_network_methods_start_no_goroutines leaves a listener, a UDP
// socket with a datagram waiting, and a dialled UDP socket open, all made
// with the four methods. synctest.Test fails with a deadlock if any of
// them left a goroutine blocked behind.
func TestHost_network_methods_start_no_goroutines(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := hosttest.New(t)
		_, err := h.ListenTCP(t.Context(), "tcp", "api:80")
		require.NoError(t, err)
		_, err = h.DialTCP(t.Context(), "tcp", "api:81")
		require.ErrorIs(t, err, syscall.ECONNREFUSED)

		_, err = h.ListenUDP(t.Context(), "udp", "dns:53")
		require.NoError(t, err)
		c, err := h.DialUDP(t.Context(), "udp", "dns:53")
		require.NoError(t, err)
		_, err = c.Write([]byte("ping"))
		require.NoError(t, err)
	})
}
