// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cart

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var (
	// ErrCartNotFound is returned when a cart ID does not exist.
	ErrCartNotFound = errors.New("cart not found")

	// ErrItemNotFound is returned when an item ID does not exist in a cart.
	ErrItemNotFound = errors.New("item not found")
)

// Item represents a product in a cart.
type Item struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

// Cart represents a shopping cart.
type Cart struct {
	ID    string `json:"id"`
	Items []Item `json:"items"`
}

// Service is an in-memory cart store.
type Service struct {
	mu    sync.Mutex
	carts map[string]*Cart
	nextID atomic.Int64
}

// NewService creates a new in-memory cart service.
func NewService() *Service {
	return &Service{
		carts: make(map[string]*Cart),
	}
}

func (s *Service) genID() string {
	return fmt.Sprintf("%d", s.nextID.Add(1))
}

// CreateCart creates a new empty cart and returns it.
func (s *Service) CreateCart() Cart {
	s.mu.Lock()
	defer s.mu.Unlock()

	c := &Cart{
		ID:    s.genID(),
		Items: []Item{},
	}
	s.carts[c.ID] = c
	return *c
}

// GetCart returns the cart with the given ID.
func (s *Service) GetCart(id string) (Cart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.carts[id]
	if !ok {
		return Cart{}, ErrCartNotFound
	}
	return *c, nil
}

// AddItem adds an item to the cart and returns the updated cart.
func (s *Service) AddItem(cartID string, name string, quantity int, price float64) (Cart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.carts[cartID]
	if !ok {
		return Cart{}, ErrCartNotFound
	}

	item := Item{
		ID:       s.genID(),
		Name:     name,
		Quantity: quantity,
		Price:    price,
	}
	c.Items = append(c.Items, item)
	return *c, nil
}

// RemoveItem removes an item from the cart and returns the updated cart.
func (s *Service) RemoveItem(cartID, itemID string) (Cart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.carts[cartID]
	if !ok {
		return Cart{}, ErrCartNotFound
	}

	for i, item := range c.Items {
		if item.ID == itemID {
			c.Items = append(c.Items[:i], c.Items[i+1:]...)
			return *c, nil
		}
	}
	return Cart{}, ErrItemNotFound
}
