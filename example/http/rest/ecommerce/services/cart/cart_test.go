// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cart

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewService(t *testing.T) {
	svc := NewService()
	assert.NotNil(t, svc)
}

func TestCreateCart(t *testing.T) {
	svc := NewService()

	c := svc.CreateCart()

	assert.NotEmpty(t, c.ID)
	assert.Empty(t, c.Items)
}

func TestCreateCart_UniqueIDs(t *testing.T) {
	svc := NewService()

	c1 := svc.CreateCart()
	c2 := svc.CreateCart()

	assert.NotEqual(t, c1.ID, c2.ID)
}

func TestGetCart(t *testing.T) {
	svc := NewService()
	created := svc.CreateCart()

	got, err := svc.GetCart(created.ID)

	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
}

func TestGetCart_NotFound(t *testing.T) {
	svc := NewService()

	_, err := svc.GetCart("nonexistent")

	assert.ErrorIs(t, err, ErrCartNotFound)
}

func TestAddItem(t *testing.T) {
	svc := NewService()
	c := svc.CreateCart()

	updated, err := svc.AddItem(c.ID, "Widget", 2, 9.99)

	require.NoError(t, err)
	require.Len(t, updated.Items, 1)
	assert.Equal(t, "Widget", updated.Items[0].Name)
	assert.Equal(t, 2, updated.Items[0].Quantity)
	assert.Equal(t, 9.99, updated.Items[0].Price)
	assert.NotEmpty(t, updated.Items[0].ID)
}

func TestAddItem_CartNotFound(t *testing.T) {
	svc := NewService()

	_, err := svc.AddItem("nonexistent", "Widget", 1, 5.00)

	assert.ErrorIs(t, err, ErrCartNotFound)
}

func TestAddItem_Multiple(t *testing.T) {
	svc := NewService()
	c := svc.CreateCart()

	svc.AddItem(c.ID, "A", 1, 1.00)
	updated, err := svc.AddItem(c.ID, "B", 2, 2.00)

	require.NoError(t, err)
	assert.Len(t, updated.Items, 2)
}

func TestRemoveItem(t *testing.T) {
	svc := NewService()
	c := svc.CreateCart()
	updated, _ := svc.AddItem(c.ID, "Widget", 1, 5.00)
	itemID := updated.Items[0].ID

	result, err := svc.RemoveItem(c.ID, itemID)

	require.NoError(t, err)
	assert.Empty(t, result.Items)
}

func TestRemoveItem_CartNotFound(t *testing.T) {
	svc := NewService()

	_, err := svc.RemoveItem("nonexistent", "item1")

	assert.ErrorIs(t, err, ErrCartNotFound)
}

func TestRemoveItem_ItemNotFound(t *testing.T) {
	svc := NewService()
	c := svc.CreateCart()

	_, err := svc.RemoveItem(c.ID, "nonexistent")

	assert.ErrorIs(t, err, ErrItemNotFound)
}

func TestRemoveItem_LeavesOtherItems(t *testing.T) {
	svc := NewService()
	c := svc.CreateCart()
	after1, _ := svc.AddItem(c.ID, "A", 1, 1.00)
	after2, _ := svc.AddItem(c.ID, "B", 2, 2.00)
	removeID := after1.Items[0].ID
	keepID := after2.Items[1].ID

	result, err := svc.RemoveItem(c.ID, removeID)

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, keepID, result.Items[0].ID)
}
