// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package host_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/z5labs/bedrock/host"
)

// Network is the network a command sees.
type Network interface {
	ListenTCP(ctx context.Context, network, address string) (net.Listener, error)
	DialTCP(ctx context.Context, network, address string) (net.Conn, error)
}

// FS is the filesystem a command sees.
type FS interface {
	Open(name string) (*os.File, error)
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	MkdirAll(name string, perm os.FileMode) error
	Chmod(name string, mode os.FileMode) error
	Getwd() (string, error)
}

// Host is what the framework hands a command.
type Host struct {
	Network    Network
	Filesystem FS
}

// OSHost returns the Host of the real machine.
func OSHost() Host {
	return Host{Network: host.IPNetwork{}, Filesystem: host.OSFilesystem{}}
}

// A CLI framework declares the network and the filesystem its commands see
// as interfaces of its own, listing the methods it hands them, and builds
// its own host struct of the two. OSHost fills it from an IPNetwork and an
// OSFilesystem, and the compiler checks that each has every method its
// interface lists. bedrock defines the two types and never passes one.
//
// The command itself reaches the filesystem only through h.Filesystem, and
// only through the methods FS lists. Only the example's setup, which
// stands in for the command's configuration, goes around it.
func Example() {
	h := OSHost()

	// The temporary directory stands in for the command's state directory,
	// which a real command would read from its configuration.
	dir, err := os.MkdirTemp("", "host-example-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()

	state := filepath.Join(dir, "state")
	if err := h.Filesystem.MkdirAll(state, 0o755); err != nil {
		fmt.Println(err)
		return
	}
	f, err := h.Filesystem.OpenFile(filepath.Join(state, "token"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		fmt.Println(err)
		return
	}
	_, err = f.WriteString("s3cr3t")
	if err := errors.Join(err, f.Close()); err != nil {
		fmt.Println(err)
		return
	}
	if err := h.Filesystem.Chmod(f.Name(), 0o600); err != nil {
		fmt.Println(err)
		return
	}

	wd, err := h.Filesystem.Getwd()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Getwd is absolute:", filepath.IsAbs(wd))

	g, err := h.Filesystem.Open(f.Name())
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = g.Close() }()
	info, err := g.Stat()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(filepath.Base(g.Name()), info.Size(), "bytes")
	// Output:
	// Getwd is absolute: true
	// token 6 bytes
}
