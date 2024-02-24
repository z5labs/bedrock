// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package health

import (
	"context"
	"sync"
)

// Metric represents anything that can report its health status.
type Metric interface {
	Healthy(context.Context) bool
}

// Binary represents a health.Metric that is either healthy or not.
// The default value is represents a healthy state.
type Binary struct {
	mu        sync.Mutex
	unhealthy bool
}

// Toggle toggles the state of Binary.
func (m *Binary) Toggle() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unhealthy = !m.unhealthy
}

// Healthy implements the Metric interface.
func (m *Binary) Healthy(ctx context.Context) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.unhealthy
}
