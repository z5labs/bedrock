// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package host

import (
	"context"
	"net"
)

// FS constrains the filesystem of a Host. Its one method is unexported, so
// FS is implemented only in this module, by OSFS and by hosttest's FS, and
// a type elsewhere satisfies it by embedding one of them. A framework
// declares the filesystem its users see as an interface that embeds FS
// and lists the methods it needs.
type FS interface {
	fs()
}

// Host is the network and the filesystem that a framework hands an App
// beyond its process. F is the filesystem the framework exposes. Every
// field is required.
//
// A server listens over TCP with Listen and over UDP with ListenPacket. A
// client connects over either with Dial, which resolves the name in the
// address over UDP as it does over TCP, so a client never resolves a UDP
// address itself.
type Host[F FS] struct {
	// Listen announces on the local network address, like net.Listen, for
	// "tcp", "tcp4" or "tcp6".
	Listen func(network, address string) (net.Listener, error)

	// ListenPacket announces on the local network address, like
	// net.ListenPacket, for "udp", "udp4" or "udp6".
	ListenPacket func(network, address string) (net.PacketConn, error)

	// Dial connects to the address on the named network, like
	// (*net.Dialer).DialContext. Over "udp", "udp4" or "udp6" it returns
	// a connected socket, which sends only to the address it dialled and
	// takes datagrams only from it.
	Dial func(ctx context.Context, network, address string) (net.Conn, error)

	// FS is the Host's filesystem.
	FS F
}

// OS returns the Host of the real machine: net.Listen, net.ListenPacket, a
// net.Dialer's DialContext, and an OSFS. A framework copies its fields
// into a Host of its own filesystem.
func OS() Host[OSFS] {
	var d net.Dialer
	return Host[OSFS]{
		Listen:       net.Listen,
		ListenPacket: net.ListenPacket,
		Dial:         d.DialContext,
		FS:           OSFS{},
	}
}
