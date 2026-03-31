// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"

	"github.com/z5labs/bedrock/example/http/rest/ecommerce/services/cart"
	"github.com/z5labs/bedrock/runtime/http/rest"
)

// CreateCart returns a Route for POST /carts.
func CreateCart(cartSvc *cart.Service) rest.Route {
	ep := rest.POST("/carts", func(_ context.Context, _ rest.Request[rest.EmptyBody]) (cart.Cart, error) {
		c := cartSvc.CreateCart()
		return c, nil
	})
	ep = rest.Summary("Create a new cart", ep)
	ep = rest.Tags([]string{"carts"}, ep)
	ep = rest.WriteJSON[cart.Cart](201, ep)
	return rest.CatchAll[InternalError](500, ep)
}
