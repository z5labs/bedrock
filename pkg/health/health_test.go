// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package health

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBinary_Toggle(t *testing.T) {
	t.Run("will make it unhealthy", func(t *testing.T) {
		t.Run("if the current state is healthy", func(t *testing.T) {
			var m Binary
			m.Toggle()
			assert.False(t, m.Healthy(context.Background()))
		})
	})

	t.Run("will make it healthy", func(t *testing.T) {
		t.Run("if the current state is unhealthy", func(t *testing.T) {
			m := Binary{
				unhealthy: true,
			}
			m.Toggle()
			assert.True(t, m.Healthy(context.Background()))
		})
	})
}

type healthyMetric bool

func (m healthyMetric) Healthy(_ context.Context) bool {
	return bool(m)
}

func TestAndMetric_Healthy(t *testing.T) {
	t.Run("will return true", func(t *testing.T) {
		testCases := []struct {
			Name    string
			Metrics []Metric
		}{
			{
				Name:    "if there is a single healthy metric",
				Metrics: []Metric{healthyMetric(true)},
			},
			{
				Name:    "if all metrics are healthy",
				Metrics: []Metric{healthyMetric(true), healthyMetric(true)},
			},
		}
		for _, testCase := range testCases {
			t.Run(testCase.Name, func(t *testing.T) {
				am := And(testCase.Metrics...)
				assert.True(t, am.Healthy(context.Background()))
			})
		}
	})

	t.Run("will return false", func(t *testing.T) {
		testCases := []struct {
			Name    string
			Metrics []Metric
		}{
			{
				Name:    "if there is a single unhealthy metric",
				Metrics: []Metric{healthyMetric(false)},
			},
			{
				Name:    "if all metrics are all unhealthy",
				Metrics: []Metric{healthyMetric(false), healthyMetric(false)},
			},
			{
				Name:    "if all one of the metrics is unhealthy",
				Metrics: []Metric{healthyMetric(true), healthyMetric(false)},
			},
			{
				Name:    "if all one of the metrics is unhealthy (symmetric)",
				Metrics: []Metric{healthyMetric(false), healthyMetric(true)},
			},
		}
		for _, testCase := range testCases {
			t.Run(testCase.Name, func(t *testing.T) {
				am := And(testCase.Metrics...)
				assert.False(t, am.Healthy(context.Background()))
			})
		}
	})
}

func TestOrMetric_Healthy(t *testing.T) {
	t.Run("will return true", func(t *testing.T) {
		testCases := []struct {
			Name    string
			Metrics []Metric
		}{
			{
				Name:    "if there is a single healthy metric",
				Metrics: []Metric{healthyMetric(true)},
			},
			{
				Name:    "if all metrics are healthy",
				Metrics: []Metric{healthyMetric(true), healthyMetric(true)},
			},
			{
				Name:    "if all one of the metrics is unhealthy",
				Metrics: []Metric{healthyMetric(true), healthyMetric(false)},
			},
			{
				Name:    "if all one of the metrics is unhealthy (symmetric)",
				Metrics: []Metric{healthyMetric(false), healthyMetric(true)},
			},
		}
		for _, testCase := range testCases {
			t.Run(testCase.Name, func(t *testing.T) {
				om := Or(testCase.Metrics...)
				assert.True(t, om.Healthy(context.Background()))
			})
		}
	})

	t.Run("will return false", func(t *testing.T) {
		testCases := []struct {
			Name    string
			Metrics []Metric
		}{
			{
				Name:    "if there is a single unhealthy metric",
				Metrics: []Metric{healthyMetric(false)},
			},
			{
				Name:    "if all metrics are all unhealthy",
				Metrics: []Metric{healthyMetric(false), healthyMetric(false)},
			},
		}
		for _, testCase := range testCases {
			t.Run(testCase.Name, func(t *testing.T) {
				om := Or(testCase.Metrics...)
				assert.False(t, om.Healthy(context.Background()))
			})
		}
	})
}
