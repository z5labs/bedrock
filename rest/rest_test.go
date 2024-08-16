// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApp_Run(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if it fails to create a listener", func(t *testing.T) {
			app := NewApp()

			listenErr := errors.New("failed to listen")
			app.listen = func(network, addr string) (net.Listener, error) {
				return nil, listenErr
			}

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, listenErr) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if the context.Context is cancelled", func(t *testing.T) {
			app := NewApp(ListenOn(0))

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := app.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
		})
	})
}
