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

// RemoveItem returns a Route for DELETE /carts/{cart_id}/items/{item_id}.
func RemoveItem(cartSvc *cart.Service) rest.Route {
	ep := rest.DELETE("/carts/{cart_id}/items/{item_id}", func(_ context.Context, req rest.Request[rest.EmptyBody]) (cart.Cart, error) {
		cID := rest.ParamFrom(req, cartID)
		iID := rest.ParamFrom(req, itemID)
		c, err := cartSvc.RemoveItem(cID, iID)
		if err != nil {
			if errors.Is(err, cart.ErrCartNotFound) || errors.Is(err, cart.ErrItemNotFound) {
				return cart.Cart{}, NotFoundError{Message: err.Error()}
			}
			return cart.Cart{}, err
		}
		return c, nil
	})
	ep = cartID.Read(ep)
	ep = itemID.Read(ep)
	ep = rest.Summary("Remove an item from a cart", ep)
	ep = rest.Tags([]string{"items"}, ep)
	ep = rest.WriteJSON[cart.Cart](200, ep)
	ep = rest.ErrorJSON[NotFoundError](404, ep)
	return rest.CatchAll[InternalError](500, wrapInternalError, ep)
}
