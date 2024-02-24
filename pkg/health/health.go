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

// AndMetric represents multiple Metrics all and'd together.
type AndMetric struct {
	metrics []Metric
}

// And returns a Metric where all the underlying Metrics healthy
// states are joined together via the logical and (&&) operator.
func And(metrics ...Metric) AndMetric {
	return AndMetric{
		metrics: metrics,
	}
}

// Healthy implements the Metric interface.
func (m AndMetric) Healthy(ctx context.Context) bool {
	for _, metric := range m.metrics {
		if !metric.Healthy(ctx) {
			return false
		}
	}
	return true
}

// OrMetric represents multiple Metrics all or'd together.
type OrMetric struct {
	metrics []Metric
}

// Or returns a Metric where all the underlying Metrics healthy
// states are joined together via the logical or (||) operator.
func Or(metrics ...Metric) OrMetric {
	return OrMetric{
		metrics: metrics,
	}
}

// Healthy implements the Metric interface.
func (m OrMetric) Healthy(ctx context.Context) bool {
	for _, metric := range m.metrics {
		if metric.Healthy(ctx) {
			return true
		}
	}
	return false
}

// NotMetric represents the not'd value of the unerlying Metric.
type NotMetric struct {
	metric Metric
}

// And returns a Metric where the underlying Metric healthy state
// is negated with the logical not (!) operator.
func Not(metric Metric) NotMetric {
	return NotMetric{
		metric: metric,
	}
}

// Healthy implements the Metric interface.
func (m NotMetric) Healthy(ctx context.Context) bool {
	return !m.metric.Healthy(ctx)
}
