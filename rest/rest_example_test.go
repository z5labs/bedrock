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

	re "github.com/z5labs/bedrock/rest/endpoint"

	"golang.org/x/sync/errgroup"
)

type echoService struct{}

type EchoRequest struct {
	Msg string `json:"msg"`
}

type EchoResponse struct {
	Msg string `json:"msg"`
}

func (echoService) Handle(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	return &EchoResponse{Msg: req.Msg}, nil
}

func Example() {
	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		fmt.Println(err)
		return
	}

	app := NewApp(
		Listener(ls),
		Title("Example"),
		Version("v0.0.0"),
		Register(Endpoint{
			Method:  http.MethodPost,
			Pattern: "/",
			Operation: re.NewOperation(
				re.ConsumesJson(
					re.ProducesJson(echoService{}),
				),
			),
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())

	eg, egctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return app.Run(egctx)
	})
	eg.Go(func() error {
		defer cancel()

		addr := ls.Addr()

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

	err = eg.Wait()
	if err != nil {
		fmt.Println(err)
		return
	}

	// Output: hello, world
}
