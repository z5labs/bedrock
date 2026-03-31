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

// AddItemRequest is the JSON body for adding an item to a cart.
type AddItemRequest struct {
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

// AddItem returns a Route for POST /carts/{cart_id}/items.
func AddItem(cartSvc *cart.Service) rest.Route {
	ep := rest.POST("/carts/{cart_id}/items", func(_ context.Context, req rest.Request[AddItemRequest]) (cart.Cart, error) {
		id := rest.ParamFrom(req, cartID)
		body := req.Body()
		c, err := cartSvc.AddItem(id, body.Name, body.Quantity, body.Price)
		if err != nil {
			if errors.Is(err, cart.ErrCartNotFound) {
				return cart.Cart{}, NotFoundError{Message: err.Error()}
			}
			return cart.Cart{}, err
		}
		return c, nil
	})
	ep = cartID.Read(ep)
	ep = rest.ReadJSON[AddItemRequest](ep)
	ep = rest.Summary("Add an item to a cart", ep)
	ep = rest.Tags([]string{"items"}, ep)
	ep = rest.WriteJSON[cart.Cart](200, ep)
	ep = rest.ErrorJSON[NotFoundError](404, ep)
	return rest.CatchAll[InternalError](500, ep)
}
