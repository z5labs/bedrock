// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import "github.com/z5labs/bedrock/runtime/http/rest"

var (
	cartID = rest.PathParam[string]("cart_id", rest.ParamDescription("The cart identifier"))
	itemID = rest.PathParam[string]("item_id", rest.ParamDescription("The item identifier"))
)
