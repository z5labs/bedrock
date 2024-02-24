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
