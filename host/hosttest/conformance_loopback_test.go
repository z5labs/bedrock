// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

//go:build conformance

package hosttest_test

import (
	"context"
	"net"
	"testing"

	"github.com/z5labs/bedrock/host"
)

// TestConformance_loopback runs the conformance scenarios on real loopback
// sockets, so it runs only with -tags conformance. It needs 127.0.0.2 on
// the loopback interface, as Linux has. It runs them on the zero
// host.IPNetwork, and the TCP and UDP tables on host.OS too, through
// osNetwork. The network rows leave host.OS out, because its Listen,
// ListenPacket and Dial pass every network through by design.
func TestConformance_loopback(t *testing.T) {
	t.Run("IPNetwork", func(t *testing.T) {
		ipNet := func(t *testing.T, scenario func(*testing.T, ipNetwork)) {
			scenario(t, host.IPNetwork{})
		}
		testConformance(t, ipNet)
		t.Run("udp", func(t *testing.T) {
			testConformanceUDP(t, ipNet)
		})
		t.Run("networks", func(t *testing.T) {
			testConformanceNetworks(t, ipNet)
		})
	})
	t.Run("OS", func(t *testing.T) {
		osNet := func(t *testing.T, scenario func(*testing.T, ipNetwork)) {
			scenario(t, osNetwork{host.OS()})
		}
		testConformance(t, osNet)
		t.Run("udp", func(t *testing.T) {
			testConformanceUDP(t, osNet)
		})
	})
}

// osNetwork adapts the Listen, ListenPacket and Dial of host.OS to the
// four methods the tables take. Its listens ignore ctx, as host.OS's
// Listen and ListenPacket take none.
type osNetwork struct {
	h host.Host[host.OSFS]
}

func (n osNetwork) ListenTCP(_ context.Context, network, address string) (net.Listener, error) {
	return n.h.Listen(network, address)
}

func (n osNetwork) DialTCP(ctx context.Context, network, address string) (net.Conn, error) {
	return n.h.Dial(ctx, network, address)
}

func (n osNetwork) ListenUDP(_ context.Context, network, address string) (net.PacketConn, error) {
	return n.h.ListenPacket(network, address)
}

func (n osNetwork) DialUDP(ctx context.Context, network, address string) (net.Conn, error) {
	return n.h.Dial(ctx, network, address)
}
