// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/z5labs/bedrock/rest/endpoint"
	"golang.org/x/sync/errgroup"
)

type echoService struct{}

type EchoRequest struct {
	Msg string `json:"msg"`
}

func (EchoRequest) ContentType() string {
	return "application/json"
}

func (req *EchoRequest) UnmarshalBinary(b []byte) error {
	return json.Unmarshal(b, req)
}

type EchoResponse struct {
	Msg string `json:"msg"`
}

func (EchoResponse) ContentType() string {
	return "application/json"
}

func (resp EchoResponse) MarshalBinary() ([]byte, error) {
	return json.Marshal(resp)
}

func (echoService) Handle(ctx context.Context, req EchoRequest) (EchoResponse, error) {
	return EchoResponse{Msg: req.Msg}, nil
}

// this is quick hack to dynamically allocate a local address
// for this example only. This is not apart of the public
// package API and instead, the option, ListenOn should be used
// to configure the HTTP server port.
func listenOnRandomPort(addrCh chan<- net.Addr) Option {
	return func(a *App) {
		a.listen = func(network, addr string) (net.Listener, error) {
			defer close(addrCh)

			ls, err := net.Listen(network, ":0")
			if err != nil {
				return nil, err
			}

			addrCh <- ls.Addr()
			return ls, nil
		}
	}
}

func Example() {
	addrCh := make(chan net.Addr)

	app := NewApp(
		listenOnRandomPort(addrCh),
		Endpoint(
			endpoint.Post(
				"/",
				echoService{},
			),
		),
	)

	ctx, cancel := context.WithCancel(context.Background())

	eg, egctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return app.Run(egctx)
	})
	eg.Go(func() error {
		defer cancel()

		// need to wait for http server to actually start
		// accepting connections on an address
		addr := <-addrCh

		resp, err := http.Post(
			fmt.Sprintf("http://%s", addr),
			"application/json",
			strings.NewReader(`{
				"msg": "hello, world"
			}`),
		)
		if err != nil {
			return err
		}

		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		var echoResp EchoResponse
		err = json.Unmarshal(b, &echoResp)
		if err != nil {
			return err
		}

		fmt.Println(echoResp.Msg)
		return nil
	})

	err := eg.Wait()
	if err != nil {
		fmt.Println(err)
		return
	}

	// Output: hello, world
}
