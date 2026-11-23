// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest

import (
	"context"
	"net"
)

// ListenTCP listens on the host and port of address, with the rules of
// [Host.Listen]. network is "tcp", "tcp4" or "tcp6"; any other network
// fails with a *net.OpError wrapping net.UnknownNetworkError.
//
// Names are compared as written and never resolved, so ctx bounds
// nothing: a listen that has begun never waits, and a done ctx doesn't
// stop it, as it doesn't stop a real listen on a literal address.
func (h *Host) ListenTCP(ctx context.Context, network, address string) (net.Listener, error) {
	if protocol(network) != "tcp" {
		return nil, unknownNetwork("listen", network)
	}
	return h.Listen(network, address)
}

// DialTCP connects to the host and port of address over "tcp", "tcp4" or
// "tcp6", with the rules [Host.Dial] has over TCP: it fails with
// ECONNREFUSED rather than wait for a listener, and returns ctx's error if
// ctx is done before the listener's Accept takes the connection. Any other
// network fails with a *net.OpError wrapping net.UnknownNetworkError. The
// method value fits http.Transport.DialContext.
func (h *Host) DialTCP(ctx context.Context, network, address string) (net.Conn, error) {
	if protocol(network) != "tcp" {
		return nil, unknownNetwork("dial", network)
	}
	return h.Dial(ctx, network, address)
}

// ListenUDP binds a UDP socket to the host and port of address, with the
// rules of [Host.ListenPacket]. network is "udp", "udp4" or "udp6"; any
// other network fails with a *net.OpError wrapping
// net.UnknownNetworkError.
//
// Names are compared as written and never resolved, so ctx bounds
// nothing, as in [Host.ListenTCP].
func (h *Host) ListenUDP(ctx context.Context, network, address string) (net.PacketConn, error) {
	if protocol(network) != "udp" {
		return nil, unknownNetwork("listen", network)
	}
	return h.ListenPacket(network, address)
}

// DialUDP returns a UDP socket connected to the host and port of address,
// with the rules [Host.Dial] has over UDP: it returns at once, whether or
// not a socket holds the address, and fails with ctx's error if ctx is
// already done. network is "udp", "udp4" or "udp6"; any other network
// fails with a *net.OpError wrapping net.UnknownNetworkError.
func (h *Host) DialUDP(ctx context.Context, network, address string) (net.Conn, error) {
	if protocol(network) != "udp" {
		return nil, unknownNetwork("dial", network)
	}
	return h.Dial(ctx, network, address)
}

// unknownNetwork is the error a method returns for a network it doesn't
// serve, the error host.IPNetwork returns.
func unknownNetwork(op, network string) error {
	return &net.OpError{Op: op, Net: network, Err: net.UnknownNetworkError(network)}
}
