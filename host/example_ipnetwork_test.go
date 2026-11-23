// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package host_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/z5labs/bedrock/host"
)

// TCPNetwork is the network an HTTP runtime uses: it listens and dials
// over TCP, and declares nothing else, so a network without TCP fails to
// compile where it's passed.
type TCPNetwork interface {
	ListenTCP(ctx context.Context, network, address string) (net.Listener, error)
	DialTCP(ctx context.Context, network, address string) (net.Conn, error)
}

// serveOnce serves one greeting on n and fetches it back through a client
// whose transport dials with n.DialTCP.
func serveOnce(ctx context.Context, n TCPNetwork) (string, error) {
	ln, err := n.ListenTCP(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "hello over TCP")
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	transport := &http.Transport{DialContext: n.DialTCP}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	body, err := fetch(ctx, client, "http://"+ln.Addr().String())
	if cerr := srv.Close(); cerr != nil {
		return "", errors.Join(err, cerr)
	}
	if serr := <-served; !errors.Is(serr, http.ErrServerClosed) {
		return "", errors.Join(err, serr)
	}
	return body, err
}

func fetch(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	b, err := io.ReadAll(resp.Body)
	if cerr := resp.Body.Close(); cerr != nil {
		return "", errors.Join(err, cerr)
	}
	return string(b), err
}

// A runtime declares the methods of IPNetwork it calls as an interface of
// its own, here TCPNetwork, and the zero IPNetwork, the real network,
// satisfies it. The DialTCP method value is an http.Transport's
// DialContext as it stands.
func ExampleIPNetwork() {
	body, err := serveOnce(context.Background(), host.IPNetwork{})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(body)
	// Output:
	// hello over TCP
}
