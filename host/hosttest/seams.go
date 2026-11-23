// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest

import (
	"context"
	"net"
)

// Seams is a network whose methods a test replaces one at a time. A method
// whose On func is set calls it, and the others call Network. Network is
// required: a method that falls back to a nil Network panics with a
// message naming Seams.Network.
//
// Go can't declare a method inside a function body, so Seams is how a test
// overrides one method inline. A test that makes every outbound TCP dial
// fail:
//
//	th := hosttest.New(t)
//	n := hosttest.Seams{Network: th}
//	n.OnDialTCP = func(context.Context, string, string) (net.Conn, error) { return nil, errReset }
//
// The On prefix is there because a field can't share a method's name.
// Seams starts no goroutine.
type Seams struct {
	Network interface {
		ListenTCP(ctx context.Context, network, address string) (net.Listener, error)
		DialTCP(ctx context.Context, network, address string) (net.Conn, error)
		ListenUDP(ctx context.Context, network, address string) (net.PacketConn, error)
		DialUDP(ctx context.Context, network, address string) (net.Conn, error)
	}
	OnListenTCP func(ctx context.Context, network, address string) (net.Listener, error)
	OnDialTCP   func(ctx context.Context, network, address string) (net.Conn, error)
	OnListenUDP func(ctx context.Context, network, address string) (net.PacketConn, error)
	OnDialUDP   func(ctx context.Context, network, address string) (net.Conn, error)
}

// ListenTCP calls OnListenTCP if it is set, and Network's ListenTCP
// otherwise.
func (s Seams) ListenTCP(ctx context.Context, network, address string) (net.Listener, error) {
	if s.OnListenTCP != nil {
		return s.OnListenTCP(ctx, network, address)
	}
	s.mustHaveNetwork("ListenTCP")
	return s.Network.ListenTCP(ctx, network, address)
}

// DialTCP calls OnDialTCP if it is set, and Network's DialTCP otherwise.
func (s Seams) DialTCP(ctx context.Context, network, address string) (net.Conn, error) {
	if s.OnDialTCP != nil {
		return s.OnDialTCP(ctx, network, address)
	}
	s.mustHaveNetwork("DialTCP")
	return s.Network.DialTCP(ctx, network, address)
}

// ListenUDP calls OnListenUDP if it is set, and Network's ListenUDP
// otherwise.
func (s Seams) ListenUDP(ctx context.Context, network, address string) (net.PacketConn, error) {
	if s.OnListenUDP != nil {
		return s.OnListenUDP(ctx, network, address)
	}
	s.mustHaveNetwork("ListenUDP")
	return s.Network.ListenUDP(ctx, network, address)
}

// DialUDP calls OnDialUDP if it is set, and Network's DialUDP otherwise.
func (s Seams) DialUDP(ctx context.Context, network, address string) (net.Conn, error) {
	if s.OnDialUDP != nil {
		return s.OnDialUDP(ctx, network, address)
	}
	s.mustHaveNetwork("DialUDP")
	return s.Network.DialUDP(ctx, network, address)
}

// mustHaveNetwork panics if Network is nil, naming the method that fell
// back to it.
func (s Seams) mustHaveNetwork(method string) {
	if s.Network == nil {
		panic("hosttest: Seams." + method + " has no On" + method + " and Seams.Network is nil; Seams.Network is required")
	}
}
