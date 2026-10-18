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
type Host[F FS] struct {
	// Listen announces on the local network address, like net.Listen.
	Listen func(network, address string) (net.Listener, error)

	// Dial connects to the address on the named network, like
	// (*net.Dialer).DialContext.
	Dial func(ctx context.Context, network, address string) (net.Conn, error)

	// FS is the Host's filesystem.
	FS F
}

// OS returns the Host of the real machine: net.Listen, a net.Dialer's
// DialContext, and an OSFS. A framework copies its fields into a Host of
// its own filesystem.
func OS() Host[OSFS] {
	var d net.Dialer
	return Host[OSFS]{
		Listen: net.Listen,
		Dial:   d.DialContext,
		FS:     OSFS{},
	}
}
