// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

//go:build conformance

package hosttest_test

import (
	"net"
	"testing"

	"github.com/z5labs/bedrock/host"
)

// TestConformance_loopback runs the conformance scenarios on real loopback
// sockets, so it runs only with -tags conformance. It needs 127.0.0.2 on
// the loopback interface, as Linux has. It runs them on host.OS and on the
// zero host.IPNetwork, which reaches the tables through ipNetworkListen
// and ipNetworkListenPacket.
func TestConformance_loopback(t *testing.T) {
	t.Run("OS", func(t *testing.T) {
		testConformance(t, func(t *testing.T, scenario func(*testing.T, listenFunc, dialFunc)) {
			h := host.OS()
			scenario(t, h.Listen, h.Dial)
		})
		t.Run("udp", func(t *testing.T) {
			testConformanceUDP(t, func(t *testing.T, scenario func(*testing.T, udpNetwork)) {
				h := host.OS()
				scenario(t, udpNetwork{listen: h.Listen, listenPacket: h.ListenPacket, dial: h.Dial})
			})
		})
	})
	t.Run("IPNetwork", func(t *testing.T) {
		testConformance(t, func(t *testing.T, scenario func(*testing.T, listenFunc, dialFunc)) {
			var n host.IPNetwork
			scenario(t, ipNetworkListen(t, n), n.DialTCP)
		})
		t.Run("udp", func(t *testing.T) {
			testConformanceUDP(t, func(t *testing.T, scenario func(*testing.T, udpNetwork)) {
				var n host.IPNetwork
				scenario(t, udpNetwork{
					listen:       ipNetworkListen(t, n),
					listenPacket: ipNetworkListenPacket(t, n),
					dial:         n.DialUDP,
				})
			})
		})
	})
}

// ipNetworkListen adapts n.ListenTCP to the tables' listenFunc, which takes
// no ctx, by passing it the test's.
func ipNetworkListen(t *testing.T, n host.IPNetwork) listenFunc {
	return func(network, address string) (net.Listener, error) {
		return n.ListenTCP(t.Context(), network, address)
	}
}

// ipNetworkListenPacket adapts n.ListenUDP to the shape of udpNetwork's
// listenPacket, which takes no ctx, by passing it the test's.
func ipNetworkListenPacket(t *testing.T, n host.IPNetwork) func(network, address string) (net.PacketConn, error) {
	return func(network, address string) (net.PacketConn, error) {
		return n.ListenUDP(t.Context(), network, address)
	}
}
