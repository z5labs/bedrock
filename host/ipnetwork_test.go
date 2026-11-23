// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package host_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host"
)

// ipNetwork lists every method of host.IPNetwork, so the assignment below
// fails to compile if IPNetwork loses one or changes its signature.
type ipNetwork interface {
	ListenTCP(ctx context.Context, network, address string) (net.Listener, error)
	DialTCP(ctx context.Context, network, address string) (net.Conn, error)
	ListenUDP(ctx context.Context, network, address string) (net.PacketConn, error)
	DialUDP(ctx context.Context, network, address string) (net.Conn, error)
}

var _ ipNetwork = host.IPNetwork{}

func TestIPNetwork_serves_HTTP_by_name(t *testing.T) {
	var n host.IPNetwork

	ln, err := n.ListenTCP(t.Context(), "tcp", "localhost:0")
	require.NoError(t, err)

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "hello")
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	defer func() {
		require.NoError(t, srv.Close())
		require.ErrorIs(t, <-served, http.ErrServerClosed)
	}()

	transport := &http.Transport{DialContext: n.DialTCP}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	url := "http://localhost:" + strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "hello", string(b))
}

func TestIPNetwork_round_trips_UDP(t *testing.T) {
	var n host.IPNetwork
	deadline := time.Now().Add(5 * time.Second)

	server, err := n.ListenUDP(t.Context(), "udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, server.Close()) }()
	require.NoError(t, server.SetDeadline(deadline))

	client, err := n.DialUDP(t.Context(), "udp", server.LocalAddr().String())
	require.NoError(t, err)
	defer func() { require.NoError(t, client.Close()) }()
	require.NoError(t, client.SetDeadline(deadline))

	_, err = client.Write([]byte("ping"))
	require.NoError(t, err)
	b := make([]byte, 64)
	m, from, err := server.ReadFrom(b)
	require.NoError(t, err)
	require.Equal(t, "ping", string(b[:m]))
	require.Equal(t, client.LocalAddr().String(), from.String())

	_, err = server.WriteTo([]byte("pong"), from)
	require.NoError(t, err)
	m, err = client.Read(b)
	require.NoError(t, err)
	require.Equal(t, "pong", string(b[:m]))
}

func TestIPNetwork_applies_one_policy_to_both_protocols(t *testing.T) {
	var (
		mu      sync.Mutex
		listens []string
		dials   []string
	)
	record := func(to *[]string) func(network, address string, c syscall.RawConn) error {
		return func(network, _ string, _ syscall.RawConn) error {
			mu.Lock()
			defer mu.Unlock()
			*to = append(*to, network)
			return nil
		}
	}
	n := host.IPNetwork{
		ListenConfig: net.ListenConfig{Control: record(&listens)},
		Dialer:       net.Dialer{Control: record(&dials)},
	}

	ln, err := n.ListenTCP(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, ln.Close()) }()

	pc, err := n.ListenUDP(t.Context(), "udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, pc.Close()) }()

	tc, err := n.DialTCP(t.Context(), "tcp", ln.Addr().String())
	require.NoError(t, err)
	defer func() { require.NoError(t, tc.Close()) }()

	uc, err := n.DialUDP(t.Context(), "udp", pc.LocalAddr().String())
	require.NoError(t, err)
	defer func() { require.NoError(t, uc.Close()) }()

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"tcp4", "udp4"}, listens)
	require.Equal(t, []string{"tcp4", "udp4"}, dials)
}

func TestIPNetwork_LocalAddr_suits_one_protocol(t *testing.T) {
	var server host.IPNetwork
	ln, err := server.ListenTCP(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, ln.Close()) }()
	pc, err := server.ListenUDP(t.Context(), "udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, pc.Close()) }()

	n := host.IPNetwork{
		Dialer: net.Dialer{LocalAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}},
	}

	c, err := n.DialTCP(t.Context(), "tcp", ln.Addr().String())
	require.NoError(t, err)
	require.NoError(t, c.Close())

	_, err = n.DialUDP(t.Context(), "udp", pc.LocalAddr().String())
	require.ErrorContains(t, err, "mismatched local address type")
}

func TestIPNetwork_refuses_other_networks(t *testing.T) {
	var n host.IPNetwork
	listenTCP := func(ctx context.Context, network, address string) error {
		ln, err := n.ListenTCP(ctx, network, address)
		require.Nil(t, ln)
		return err
	}
	dialTCP := func(ctx context.Context, network, address string) error {
		c, err := n.DialTCP(ctx, network, address)
		require.Nil(t, c)
		return err
	}
	listenUDP := func(ctx context.Context, network, address string) error {
		pc, err := n.ListenUDP(ctx, network, address)
		require.Nil(t, pc)
		return err
	}
	dialUDP := func(ctx context.Context, network, address string) error {
		c, err := n.DialUDP(ctx, network, address)
		require.Nil(t, c)
		return err
	}

	testCases := []struct {
		name    string
		call    func(ctx context.Context, network, address string) error
		op      string
		network string
		address string
	}{
		{name: "ListenTCP udp", call: listenTCP, op: "listen", network: "udp", address: "127.0.0.1:0"},
		{name: "ListenTCP unix", call: listenTCP, op: "listen", network: "unix", address: "sock"},
		{name: "DialTCP udp", call: dialTCP, op: "dial", network: "udp", address: "127.0.0.1:9"},
		{name: "DialTCP unix", call: dialTCP, op: "dial", network: "unix", address: "sock"},
		{name: "ListenUDP tcp", call: listenUDP, op: "listen", network: "tcp", address: "127.0.0.1:0"},
		{name: "ListenUDP unixgram", call: listenUDP, op: "listen", network: "unixgram", address: "sock"},
		{name: "DialUDP tcp", call: dialUDP, op: "dial", network: "tcp", address: "127.0.0.1:9"},
		{name: "DialUDP unixgram", call: dialUDP, op: "dial", network: "unixgram", address: "sock"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(t.Context(), tc.network, tc.address)

			var opErr *net.OpError
			require.True(t, errors.As(err, &opErr), "%v is not a *net.OpError", err)
			require.Equal(t, &net.OpError{
				Op:  tc.op,
				Net: tc.network,
				Err: net.UnknownNetworkError(tc.network),
			}, opErr)

			var unknown net.UnknownNetworkError
			require.ErrorAs(t, err, &unknown)
			require.Equal(t, net.UnknownNetworkError(tc.network), unknown)
		})
	}
}
