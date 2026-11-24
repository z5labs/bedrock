// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

//go:build conformance

package hosttest_test

import (
	"testing"

	"github.com/z5labs/bedrock/host"
)

// TestConformance_loopback runs the conformance scenarios on real loopback
// sockets, so it runs only with -tags conformance. It needs 127.0.0.2 on
// the loopback interface, as Linux has. It runs every table, the TCP, UDP
// and network rows alike, on the zero host.IPNetwork, through the four
// methods.
func TestConformance_loopback(t *testing.T) {
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
}
