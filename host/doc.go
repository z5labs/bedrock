// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package host is the network and the filesystem that a framework hands an
// App beyond its process.
//
// # What a host is
//
// bedrock hands an App what exec hands a process: its arguments, its
// environment, its standard streams and its signals. The host is the rest
// of the machine an App reaches: the network it listens on and dials, and
// the filesystem it opens files in. This package defines two types for
// them, [IPNetwork], the machine's TCP and UDP, and [OSFilesystem], its
// filesystem. bedrock defines the types and never passes one. Each
// framework built on bedrock decides how an App receives them, such as a
// field of the value the framework hands each command or service.
//
// Package [github.com/z5labs/bedrock/host/hosttest] gives every framework
// what a test hands in their place: a *hosttest.Host, whose in-memory
// network has the four methods of IPNetwork, and its Filesystem(), a
// filesystem with the methods of OSFilesystem confined to the test's
// directory.
//
// # The network
//
// An [IPNetwork] carries TCP and UDP, through four methods. A server
// listens over TCP with [IPNetwork.ListenTCP], which returns a
// [net.Listener], and over UDP with [IPNetwork.ListenUDP], which returns a
// [net.PacketConn], because UDP has no listener. A client connects over
// TCP with [IPNetwork.DialTCP], and over UDP with [IPNetwork.DialUDP],
// which returns a connected socket: one that sends only to the address it
// dialled and takes datagrams only from it. DialUDP resolves the name in a
// UDP address as DialTCP does over TCP, so a client never resolves a UDP
// address itself. A protocol over UDP, such as HTTP/3 over QUIC, serves on
// a socket from ListenUDP, and its client dials with DialUDP.
//
// An IPNetwork holds one [net.ListenConfig] and one [net.Dialer], and
// each sets the policy for both protocols: a Dialer's Resolver, Timeout
// or Control applies to DialTCP and DialUDP alike. A Dialer's LocalAddr is
// the exception. It is a *net.TCPAddr or a *net.UDPAddr, so it suits one
// protocol, and an App that pins a local address builds one IPNetwork for
// each protocol.
//
// Each method checks its network argument: the TCP methods take "tcp",
// "tcp4" or "tcp6", the UDP methods "udp", "udp4" or "udp6", and any
// other network fails with a *net.OpError wrapping
// [net.UnknownNetworkError]. So unix sockets are refused, by IPNetwork and
// by hosttest alike. An App that dials one does so with its own
// [net.Dialer].
//
// # bedrock fixes the contract, and each framework chooses the surface
//
// The two types fix the contract, while the methods an App sees are the
// framework's choice:
//
//   - A framework builds its own host struct, of a network and a
//     filesystem, each typed as an interface of its own that lists the
//     methods it hands its users. Its main fills it from an [IPNetwork] and
//     an [OSFilesystem], and its tests from a *hosttest.Host and its
//     Filesystem().
//   - Every consumer, framework or App, declares the methods it calls, as a
//     narrower interface of its own, rather than taking more than it uses.
//   - Every assignment is checked when it compiles: an IPNetwork, an
//     OSFilesystem, or one of hosttest's, is assigned to the framework's
//     interface, and a method the interface lists that it lacks fails the
//     build, not a test.
//   - A filesystem written from scratch is a filesystem: any type with the
//     methods an interface lists satisfies it, and nothing has to be
//     embedded.
//
// The shape, for a framework whose package is app:
//
//	// main
//	h := app.Host{Network: host.IPNetwork{}, Filesystem: host.OSFilesystem{}}
//
//	// a test
//	th := hosttest.New(t, hosttest.WithFiles(map[string][]byte{"etc/app.yaml": cfg}))
//	h := app.Host{Network: th, Filesystem: th.Filesystem()}
//
//	// a test that makes every outbound dial fail, inline
//	n := hosttest.Seams{Network: th}
//	n.OnDialTCP = func(context.Context, string, string) (net.Conn, error) { return nil, errReset }
//	h := app.Host{Network: n, Filesystem: th.Filesystem()}
//
//	// a runtime declares what it calls
//	func NewServer(n interface {
//		ListenTCP(ctx context.Context, network, address string) (net.Listener, error)
//	}) *Server
//
// A filesystem that confines another embeds nothing. It holds the
// filesystem it confines in a field typed as the framework's interface,
// and writes every method it offers itself, each checking the name and
// then calling that field. Embedding an OSFilesystem would promote every
// method it forgot to write, and each of those would be the real
// filesystem's, reaching around the confinement. Embedding nothing
// promotes nothing, so a forgotten method fails to compile where the
// filesystem is assigned to the framework's interface. A confining
// filesystem for a framework's FS starts like this, and assigning a
// confined to FS fails to compile until it has every method FS lists:
//
//	type confined struct {
//		inner FS     // the filesystem it confines
//		dir   string // the directory it confines inner to
//	}
//
//	func (c confined) Open(name string) (*os.File, error) {
//		if !c.inside(name) {
//			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
//		}
//		return c.inner.Open(name)
//	}
//
// # Declaring a framework's filesystem
//
// A framework's filesystem interface lists the methods of [OSFilesystem]
// it hands its users, with the same signatures, as the framework example
// shows. hosttest's *Filesystem has the same methods, so it satisfies the
// interface too. To offer more than OSFilesystem has, a framework extends
// it by wrapping it: a struct that embeds OSFilesystem and adds methods of
// its own. hosttest's *Filesystem then lacks those methods, so the
// framework's tests wrap *Filesystem the same way, and a method written
// only in terms of the methods both have can be shared by the two
// wrappers.
//
// A filesystem interface can't embed the [io/fs] interfaces, such as
// [io/fs.ReadFileFS] or [io/fs.StatFS]. Each of them embeds [io/fs.FS],
// whose Open returns an [io/fs.File], while the Open of OSFilesystem and
// of hosttest's *Filesystem returns an *[os.File]. An interface that also
// lists that Open fails to compile with "duplicate method Open", and one
// that doesn't is satisfied by neither filesystem. [OSFilesystem.DirFS]
// is the way into io/fs: it returns an [io/fs.FS] of a directory, for
// [io/fs.WalkDir], [io/fs.Glob], template.ParseFS and the rest of the
// standard library that reads one.
//
// # Relative names and Getwd
//
// A relative name is relative to the filesystem's Getwd,
// [OSFilesystem.Getwd] for the real machine and the test's directory for
// hosttest. There is no Chdir: the working directory is process-wide
// state, and a filesystem whose Getwd could change under it would make
// every relative name ambiguous.
//
// [path/filepath.Abs] reads the real working directory, not the
// filesystem's, so under hosttest it returns a path outside the test's
// directory. Code that needs an absolute path keeps a name that is
// already absolute, and joins a relative one to the filesystem's Getwd
// with [path/filepath.Join] instead.
//
// # Ambient leaks
//
// Some of the standard library reads the real machine behind the
// network's and the filesystem's back. Avoid it, or configure it from
// them:
//
//   - [os.Getwd] and [path/filepath.Abs] read the real working directory.
//     Use the filesystem's Getwd.
//   - [net/http.DefaultTransport] and [net.DefaultResolver] dial the real
//     network. Build a [net/http.Transport] whose DialContext is the
//     network's DialTCP method value instead.
//   - A program started with [os/exec] sees the real network and the whole
//     filesystem, whatever the App was handed.
//
// The design is discussion #614,
// https://github.com/z5labs/bedrock/discussions/614, and discussion #653,
// https://github.com/z5labs/bedrock/discussions/653.
package host
