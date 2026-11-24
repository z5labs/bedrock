// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"

	"github.com/z5labs/bedrock/host/hosttest"
)

// Network is the network fetch dials.
type Network interface {
	DialTCP(ctx context.Context, network, address string) (net.Conn, error)
}

// FS is the filesystem fetch writes to.
type FS interface {
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
}

// AppHost is the host struct a framework hands fetch: its own, of the
// methods fetch calls.
type AppHost struct {
	Network    Network
	Filesystem FS
}

// fetch downloads url into the file named name.
func fetch(ctx context.Context, h AppHost, url, name string) error {
	tr := &http.Transport{DialContext: h.Network.DialTCP}
	defer tr.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	f, err := h.Filesystem.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Transport: tr}).Do(req)
	if err != nil {
		return errors.Join(err, f.Close())
	}
	_, err = io.Copy(f, resp.Body)
	return errors.Join(err, resp.Body.Close(), f.Close())
}

// fetch is code a framework would hand an AppHost built from host.IPNetwork{}
// and host.OSFilesystem{}. A test hands it one built from a hosttest Host
// and its Filesystem instead, so the server it downloads from listens on
// the Host's in-memory network, and the file it writes lands in the test's
// own directory. The test reads that file back through Path, which gives
// its absolute OS path.
func ExampleNew() {
	t := new(exampleTB)
	defer t.runCleanups()

	h := hosttest.New(t)
	ln, err := h.ListenTCP(context.Background(), "tcp", "downloads.internal:80")
	if err != nil {
		fmt.Println(err)
		return
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "hello")
	})}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	hh := AppHost{Network: h, Filesystem: h.Filesystem()}
	err = fetch(context.Background(), hh, "http://downloads.internal/hello.txt", "hello.txt")
	if err != nil {
		fmt.Println(err)
		return
	}

	b, err := os.ReadFile(h.Path("hello.txt"))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(b))

	if err := srv.Close(); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("server closed:", errors.Is(<-served, http.ErrServerClosed))
	// Output:
	// hello
	// server closed: true
}
