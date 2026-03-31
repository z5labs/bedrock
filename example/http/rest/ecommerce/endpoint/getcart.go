// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"errors"

	"github.com/z5labs/bedrock/example/http/rest/ecommerce/services/cart"
	"github.com/z5labs/bedrock/runtime/http/rest"
)

// GetCart returns a Route for GET /carts/{cart_id}.
func GetCart(cartSvc *cart.Service) rest.Route {
	ep := rest.GET("/carts/{cart_id}", func(_ context.Context, req rest.Request[rest.EmptyBody]) (cart.Cart, error) {
		id := rest.ParamFrom(req, cartID)
		c, err := cartSvc.GetCart(id)
		if err != nil {
			if errors.Is(err, cart.ErrCartNotFound) {
				return cart.Cart{}, NotFoundError{Message: err.Error()}
			}
			return cart.Cart{}, err
		}
		return c, nil
	})
	ep = cartID.Read(ep)
	ep = rest.Summary("Get a cart by ID", ep)
	ep = rest.Tags([]string{"carts"}, ep)
	ep = rest.WriteJSON[cart.Cart](200, ep)
	ep = rest.ErrorJSON[NotFoundError](404, ep)
	return rest.CatchAll[InternalError](500, ep)
}
