// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	re "github.com/z5labs/bedrock/rest/endpoint"

	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go/openapi3"
	"golang.org/x/sync/errgroup"
)

type echoService struct{}

type EchoRequest struct {
	Msg string `json:"msg"`
}

func (EchoRequest) ContentType() string {
	return "application/json"
}

func (EchoRequest) Validate() error {
	return nil
}

func (req EchoRequest) OpenApiV3Schema() (*openapi3.Schema, error) {
	var reflector jsonschema.Reflector
	jsonSchema, err := reflector.Reflect(req)
	if err != nil {
		return nil, err
	}
	var schemaOrRef openapi3.SchemaOrRef
	schemaOrRef.FromJSONSchema(jsonSchema.ToSchemaOrBool())
	return schemaOrRef.Schema, nil
}

func (req *EchoRequest) ReadFrom(r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	err = json.Unmarshal(b, &req)
	return int64(len(b)), err
}

type EchoResponse struct {
	Msg string `json:"msg"`
}

func (EchoResponse) ContentType() string {
	return "application/json"
}

func (resp EchoResponse) OpenApiV3Schema() (*openapi3.Schema, error) {
	var reflector jsonschema.Reflector
	jsonSchema, err := reflector.Reflect(resp)
	if err != nil {
		return nil, err
	}
	var schemaOrRef openapi3.SchemaOrRef
	schemaOrRef.FromJSONSchema(jsonSchema.ToSchemaOrBool())
	return schemaOrRef.Schema, nil
}

func (resp *EchoResponse) WriteTo(w io.Writer) (int64, error) {
	b, err := json.Marshal(resp)
	if err != nil {
		return 0, err
	}
	return io.Copy(w, bytes.NewReader(b))
}

func (resp EchoResponse) MarshalBinary() ([]byte, error) {
	return json.Marshal(resp)
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
		Endpoint(
			http.MethodPost,
			"/",
			re.NewOperation(
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
