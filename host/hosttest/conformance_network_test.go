// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// testConformanceNetworks runs the scenarios about the network argument,
// which the four methods of a Host must get right exactly as
// host.IPNetwork does. TestConformance_in_memory runs them on a Host, and
// TestConformance_loopback on the zero host.IPNetwork. Neither runs them
// on host.OS or on a Host's Listen, ListenPacket and Dial, which pass
// every network through by design.
//
// Each method refuses the other protocol's networks, and the two listens
// ignore a done ctx: a listen on a literal address resolves nothing for
// ctx to bound, and on go1.27.0 a real one succeeds with a cancelled ctx.
func testConformanceNetworks(t *testing.T, run func(t *testing.T, scenario func(t *testing.T, n ipNetwork))) {
	refusals := []struct {
		method  string
		network string
		call    func(ctx context.Context, n ipNetwork, network string) (io.Closer, error)
	}{
		{method: "DialTCP", network: "udp", call: callDialTCP},
		{method: "DialTCP", network: "unix", call: callDialTCP},
		{method: "ListenTCP", network: "udp", call: callListenTCP},
		{method: "ListenTCP", network: "unix", call: callListenTCP},
		{method: "DialUDP", network: "tcp", call: callDialUDP},
		{method: "DialUDP", network: "unixgram", call: callDialUDP},
		{method: "ListenUDP", network: "tcp", call: callListenUDP},
		{method: "ListenUDP", network: "unixgram", call: callListenUDP},
	}
	for _, tc := range refusals {
		t.Run(tc.method+" refuses "+tc.network, func(t *testing.T) {
			run(t, func(t *testing.T, n ipNetwork) {
				c, err := tc.call(t.Context(), n, tc.network)
				if c != nil {
					_ = c.Close()
				}
				require.Error(t, err)
				unknown, ok := errors.AsType[net.UnknownNetworkError](err)
				require.True(t, ok, "%v is not a net.UnknownNetworkError", err)
				require.Equal(t, net.UnknownNetworkError(tc.network), unknown)
				opErr, ok := errors.AsType[*net.OpError](err)
				require.True(t, ok, "%T is not a *net.OpError", err)
				op := "listen"
				if tc.method == "DialTCP" || tc.method == "DialUDP" {
					op = "dial"
				}
				require.Equal(t, op, opErr.Op)
				require.Equal(t, tc.network, opErr.Net)
			})
		})
	}

	listens := []struct {
		method  string
		network string
		call    func(ctx context.Context, n ipNetwork, network string) (io.Closer, error)
	}{
		{method: "ListenTCP", network: "tcp", call: callListenTCP},
		{method: "ListenUDP", network: "udp", call: callListenUDP},
	}
	for _, tc := range listens {
		t.Run(tc.method+" succeeds with a cancelled ctx", func(t *testing.T) {
			run(t, func(t *testing.T, n ipNetwork) {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()

				c, err := tc.call(ctx, n, tc.network)
				require.NoError(t, err)
				require.NoError(t, c.Close())
			})
		})
	}
}

// The calls the network rows make, each on 127.0.0.1:0, so a call that
// wrongly succeeds holds a free port and nothing else. Each returns a nil
// io.Closer when its call fails.

func callDialTCP(ctx context.Context, n ipNetwork, network string) (io.Closer, error) {
	c, err := n.DialTCP(ctx, network, "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return c, nil
}

func callListenTCP(ctx context.Context, n ipNetwork, network string) (io.Closer, error) {
	ln, err := n.ListenTCP(ctx, network, "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return ln, nil
}

func callDialUDP(ctx context.Context, n ipNetwork, network string) (io.Closer, error) {
	c, err := n.DialUDP(ctx, network, "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return c, nil
}

func callListenUDP(ctx context.Context, n ipNetwork, network string) (io.Closer, error) {
	pc, err := n.ListenUDP(ctx, network, "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return pc, nil
}
