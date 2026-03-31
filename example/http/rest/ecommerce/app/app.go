// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"
	"net"
	"time"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"
	"github.com/z5labs/bedrock/example/http/rest/ecommerce/endpoint"
	"github.com/z5labs/bedrock/example/http/rest/ecommerce/services/cart"
	httprt "github.com/z5labs/bedrock/runtime/http"
	"github.com/z5labs/bedrock/runtime/http/rest"
)

// New constructs a bedrock.Builder for the ecommerce HTTP runtime.
func New(cartSvc *cart.Service) bedrock.Builder[httprt.Runtime] {
	handler := rest.Build(
		rest.Title("Ecommerce Cart API"),
		rest.Version("1.0.0"),
		rest.APIDescription("A simple ecommerce cart CRUD API"),
		endpoint.CreateCart(cartSvc).Route(),
		endpoint.GetCart(cartSvc).Route(),
		endpoint.AddItem(cartSvc).Route(),
		endpoint.RemoveItem(cartSvc).Route(),
	)

	listener := bedrock.Map(
		httprt.BuildTCPListener(config.ReaderOf(&net.TCPAddr{Port: 8080})),
		func(_ context.Context, ln *net.TCPListener) (net.Listener, error) {
			return ln, nil
		},
	)

	return httprt.Build(
		listener,
		handler,
		httprt.DisableGeneralOptionsHandler(config.ReaderOf(false)),
		httprt.ReadTimeout(config.ReaderOf(5*time.Second)),
		httprt.ReadHeaderTimeout(config.ReaderOf(2*time.Second)),
		httprt.WriteTimeout(config.ReaderOf(10*time.Second)),
		httprt.IdleTimeout(config.ReaderOf(120*time.Second)),
		httprt.MaxHeaderBytes(config.ReaderOf(1048576)),
	)
}
