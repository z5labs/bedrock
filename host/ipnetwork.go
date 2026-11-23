// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package host

import (
	"context"
	"net"
)

// IPNetwork is the machine's TCP and UDP. Its zero value is the real
// network with Go's defaults.
//
// ListenConfig and Dialer set the policy for both protocols. A Dialer's
// Resolver, Timeout or Control applies to DialTCP and DialUDP alike, and
// ListenConfig applies to ListenTCP and ListenUDP.
//
// A Dialer's LocalAddr is the exception. It is a *net.TCPAddr or a
// *net.UDPAddr, so it suits one protocol, and a dial over the other fails
// with "mismatched local address type". To pin a local address, build one
// IPNetwork for each protocol.
//
// A consumer declares the methods it calls, for example
//
//	type TCPNetwork interface {
//		ListenTCP(ctx context.Context, network, address string) (net.Listener, error)
//		DialTCP(ctx context.Context, network, address string) (net.Conn, error)
//	}
type IPNetwork struct {
	ListenConfig net.ListenConfig
	Dialer       net.Dialer
}

// ListenTCP announces on address, like net.ListenConfig.Listen. ctx bounds
// the resolution of a name in address; it doesn't close the listener.
// network is "tcp", "tcp4" or "tcp6". Any other network is an error.
func (n IPNetwork) ListenTCP(ctx context.Context, network, address string) (net.Listener, error) {
	if !isTCP(network) {
		return nil, unknownNetwork("listen", network)
	}
	return n.ListenConfig.Listen(ctx, network, address)
}

// DialTCP connects to address, like net.Dialer.DialContext. network is
// "tcp", "tcp4" or "tcp6". The method value fits http.Transport.DialContext.
func (n IPNetwork) DialTCP(ctx context.Context, network, address string) (net.Conn, error) {
	if !isTCP(network) {
		return nil, unknownNetwork("dial", network)
	}
	return n.Dialer.DialContext(ctx, network, address)
}

// ListenUDP binds address, like net.ListenConfig.ListenPacket. network is
// "udp", "udp4" or "udp6".
func (n IPNetwork) ListenUDP(ctx context.Context, network, address string) (net.PacketConn, error) {
	if !isUDP(network) {
		return nil, unknownNetwork("listen", network)
	}
	return n.ListenConfig.ListenPacket(ctx, network, address)
}

// DialUDP returns a UDP socket connected to address, like
// net.Dialer.DialContext. network is "udp", "udp4" or "udp6".
func (n IPNetwork) DialUDP(ctx context.Context, network, address string) (net.Conn, error) {
	if !isUDP(network) {
		return nil, unknownNetwork("dial", network)
	}
	return n.Dialer.DialContext(ctx, network, address)
}

func isTCP(network string) bool {
	return network == "tcp" || network == "tcp4" || network == "tcp6"
}

func isUDP(network string) bool {
	return network == "udp" || network == "udp4" || network == "udp6"
}

// unknownNetwork is the error a method returns for a network it doesn't
// serve, in the shape hosttest's errors have: no Source and no Addr.
func unknownNetwork(op, network string) error {
	return &net.OpError{Op: op, Net: network, Err: net.UnknownNetworkError(network)}
}
