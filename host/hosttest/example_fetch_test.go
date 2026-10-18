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
	"net/http"
	"os"

	"github.com/z5labs/bedrock/host"
	"github.com/z5labs/bedrock/host/hosttest"
)

// FS is the filesystem fetch writes to.
type FS interface {
	host.FS
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
}

// fetch downloads url into the file named name.
func fetch(ctx context.Context, h host.Host[FS], url, name string) error {
	tr := &http.Transport{DialContext: h.Dial}
	defer tr.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	f, err := h.FS.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
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

// fetch is code a framework would hand a host.Host[FS] of the real
// machine. A test hands it one built from a hosttest Host instead, so the
// server it downloads from listens on the Host's in-memory network, and the
// file it writes lands in the test's own directory. The test reads that
// file back through Path, which gives its absolute OS path.
func ExampleNew() {
	t := new(exampleTB)
	defer t.runCleanups()

	h := hosttest.New(t)
	ln, err := h.Listen("tcp", "downloads.internal:80")
	if err != nil {
		fmt.Println(err)
		return
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "hello")
	})}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	hh := host.Host[FS]{Listen: h.Listen, Dial: h.Dial, FS: h.FS()}
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
