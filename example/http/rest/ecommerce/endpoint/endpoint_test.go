// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/z5labs/bedrock/example/http/rest/ecommerce/services/cart"
	"github.com/z5labs/bedrock/runtime/http/rest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildHandler(t *testing.T, routes ...rest.Route) http.Handler {
	t.Helper()

	opts := []rest.Option{rest.Title("Test"), rest.Version("1.0.0")}
	for _, r := range routes {
		opts = append(opts, r.Route())
	}

	handler := rest.Build(opts...)
	h, err := handler.Build(context.Background())
	require.NoError(t, err)
	return h
}

func TestCreateCart(t *testing.T) {
	svc := cart.NewService()
	h := buildHandler(t, CreateCart(svc))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/carts", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)

	var c cart.Cart
	err := json.Unmarshal(w.Body.Bytes(), &c)
	require.NoError(t, err)
	assert.NotEmpty(t, c.ID)
	assert.Empty(t, c.Items)
}

func TestGetCart(t *testing.T) {
	svc := cart.NewService()
	created := svc.CreateCart()
	h := buildHandler(t, GetCart(svc))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/carts/"+created.ID, nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var c cart.Cart
	err := json.Unmarshal(w.Body.Bytes(), &c)
	require.NoError(t, err)
	assert.Equal(t, created.ID, c.ID)
}

func TestGetCart_NotFound(t *testing.T) {
	svc := cart.NewService()
	h := buildHandler(t, GetCart(svc))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/carts/nonexistent", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var errResp NotFoundError
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Contains(t, errResp.Message, "cart not found")
}

func TestAddItem(t *testing.T) {
	svc := cart.NewService()
	created := svc.CreateCart()
	h := buildHandler(t, AddItem(svc))

	body := `{"name":"Widget","quantity":2,"price":9.99}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/carts/"+created.ID+"/items", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var c cart.Cart
	err := json.Unmarshal(w.Body.Bytes(), &c)
	require.NoError(t, err)
	require.Len(t, c.Items, 1)
	assert.Equal(t, "Widget", c.Items[0].Name)
	assert.Equal(t, 2, c.Items[0].Quantity)
	assert.Equal(t, 9.99, c.Items[0].Price)
}

func TestAddItem_CartNotFound(t *testing.T) {
	svc := cart.NewService()
	h := buildHandler(t, AddItem(svc))

	body := `{"name":"Widget","quantity":1,"price":5.00}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/carts/nonexistent/items", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRemoveItem(t *testing.T) {
	svc := cart.NewService()
	created := svc.CreateCart()
	updated, _ := svc.AddItem(created.ID, "Widget", 1, 5.00)
	itemID := updated.Items[0].ID
	h := buildHandler(t, RemoveItem(svc))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/carts/"+created.ID+"/items/"+itemID, nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var c cart.Cart
	err := json.Unmarshal(w.Body.Bytes(), &c)
	require.NoError(t, err)
	assert.Empty(t, c.Items)
}

func TestRemoveItem_CartNotFound(t *testing.T) {
	svc := cart.NewService()
	h := buildHandler(t, RemoveItem(svc))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/carts/nonexistent/items/item1", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRemoveItem_ItemNotFound(t *testing.T) {
	svc := cart.NewService()
	created := svc.CreateCart()
	h := buildHandler(t, RemoveItem(svc))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/carts/"+created.ID+"/items/nonexistent", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
