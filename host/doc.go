// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package host is the network and the filesystem that a framework hands an
// App beyond its process.
//
// # What a Host is
//
// bedrock hands an App what exec hands a process: its arguments, its
// environment, its standard streams and its signals. A [Host] is the rest of
// the machine an App reaches: the network it listens on and dials, and the
// filesystem it opens files in. bedrock defines the Host and never passes
// one. Each framework built on bedrock decides how an App receives it, such
// as a field of the value the framework hands each command or service.
//
// This module gives every framework the same two implementations: [OS],
// the Host of the real machine, and the Host of package
// [github.com/z5labs/bedrock/host/hosttest], an in-memory network and a
// filesystem confined to a test's directory.
//
// # bedrock fixes the contract, and each framework chooses the surface
//
// [Host] is generic over its filesystem, so the contract is fixed while the
// methods an App sees are the framework's choice:
//
//   - A framework declares an interface that embeds [FS] and lists the
//     methods it needs, and then aliases type Host = host.Host[FS], so its
//     users never write the type parameter.
//   - Every consumer, framework or App, declares the methods it calls, as a
//     narrower interface of its own, rather than taking more than it uses.
//   - Every assignment is checked when it compiles: an [OSFS], or hosttest's
//     *FS, is assigned to the framework's interface, and a method the
//     interface lists that the filesystem lacks fails the build, not a test.
//
// # FS is sealed
//
// [FS] has one unexported method, so only this module implements it: [OSFS]
// and hosttest's *FS are the only filesystems. A type elsewhere satisfies FS
// by embedding one of them, or FS itself:
//
//   - A test changes one method by embedding [OSFS], or hosttest's *FS, and
//     overriding that method, for example to make OpenFile fail with a full
//     disk.
//   - A filesystem that confines another embeds [FS] and never [OSFS]. It
//     embeds FS only for the seal, holds the filesystem it confines in a
//     field of its own, typed as the framework's interface, and writes
//     every method it offers itself, each checking the name and then
//     calling that field. Embedding OSFS would promote every method it
//     forgot to write, and each of those would be the real filesystem's,
//     reaching around the confinement. Embedding FS promotes no method but
//     the unexported one, so a forgotten method fails to compile where the
//     filesystem is assigned to the framework's interface.
//
// A confining filesystem for the framework example's FS starts like this,
// and assigning a confined to FS fails to compile until it has every method
// FS lists:
//
//	type confined struct {
//		host.FS        // the seal, and nothing else
//		inner   FS     // the filesystem it confines
//		dir     string // the directory it confines inner to
//	}
//
//	func (c confined) Open(name string) (*os.File, error) {
//		if !c.inside(name) {
//			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
//		}
//		return c.inner.Open(name)
//	}
//
// # Declaring a framework's FS
//
// A framework's FS lists the methods of [OSFS] it hands its users, with
// the same signatures, as the framework example shows. To offer more than
// OSFS has, it extends OSFS by wrapping it: a struct that embeds OSFS and
// adds methods of its own. hosttest's *FS then lacks those methods, so the
// framework's tests wrap *FS the same way, and a method written only in
// terms of the methods both have can be shared by the two wrappers.
//
// An FS can't embed the [io/fs] interfaces, such as [io/fs.ReadFileFS] or
// [io/fs.StatFS]. Each of them embeds [io/fs.FS], whose Open returns an
// [io/fs.File], while the Open of OSFS and of hosttest's *FS returns an
// *[os.File]. An FS that also lists that Open fails to compile with
// "duplicate method Open", and one that doesn't is satisfied by neither
// filesystem. [OSFS.DirFS] is the way into io/fs: it returns an
// [io/fs.FS] of a directory, for [io/fs.WalkDir], [io/fs.Glob],
// template.ParseFS and the rest of the standard library that reads one.
//
// # Relative names and Getwd
//
// A relative name is relative to the filesystem's Getwd, [OSFS.Getwd] for
// the real machine and the test's directory for hosttest. There is no
// Chdir: the working directory is process-wide state, and a filesystem
// whose Getwd could change under it would make every relative name
// ambiguous.
//
// [path/filepath.Abs] reads the real working directory, not the
// filesystem's, so under hosttest it returns a path outside the test's
// directory. Code that needs an absolute path keeps a name that is
// already absolute, and joins a relative one to the filesystem's Getwd
// with [path/filepath.Join] instead.
//
// # Ambient leaks
//
// Some of the standard library reads the real machine behind the Host's
// back. Avoid it, or configure it from the Host:
//
//   - [os.Getwd] and [path/filepath.Abs] read the real working directory.
//     Use the filesystem's Getwd.
//   - [net/http.DefaultTransport] and [net.DefaultResolver] dial the real
//     network. Build a [net/http.Transport] whose DialContext is the Host's
//     Dial instead.
//   - A program started with [os/exec] sees the real network and the whole
//     filesystem, whatever the Host is.
package host
