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
	"syscall"

	"github.com/z5labs/bedrock/host/hosttest"
)

// A test makes every outbound TCP dial fail in one statement of its own
// body. The Seams calls OnDialTCP for DialTCP, and the Host for every other
// method, so the code under test still listens on the Host while each
// request it makes fails with the injected error.
func ExampleSeams() {
	t := new(exampleTB)
	defer t.runCleanups()

	th := hosttest.New(t)
	errReset := &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNRESET)}
	n := hosttest.Seams{Network: th}
	n.OnDialTCP = func(context.Context, string, string) (net.Conn, error) { return nil, errReset }

	// The code under test is handed n, and listens on it as it would on
	// the real machine.
	ln, err := n.ListenTCP(context.Background(), "tcp", "api.internal:80")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = ln.Close() }()
	fmt.Println("listening on", ln.Addr())

	// Every request it makes dials with n.DialTCP, and fails.
	tr := &http.Transport{DialContext: n.DialTCP}
	defer tr.CloseIdleConnections()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://api.internal/", nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	resp, err := (&http.Client{Transport: tr}).Do(req)
	if err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	fmt.Println("connection reset:", errors.Is(err, syscall.ECONNRESET))
	// Output:
	// listening on api.internal:80
	// connection reset: true
}
