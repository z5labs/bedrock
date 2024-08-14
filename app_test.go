// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/z5labs/bedrock/pkg/config"
)

type configSourceFunc func(config.Store) error

func (f configSourceFunc) Apply(store config.Store) error {
	return f(store)
}

func TestRun(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the config.Source(s) fail to be read", func(t *testing.T) {
			srcErr := errors.New("failed to apply config")
			src := configSourceFunc(func(s config.Store) error {
				return srcErr
			})

			type myConfig struct{}
			b := AppBuilderFunc[myConfig](func(ctx context.Context, cfg myConfig) (App, error) {
				return nil, nil
			})

			err := Run(context.Background(), b, src)

			var ierr ConfigReadError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
			if !assert.ErrorIs(t, ierr, srcErr) {
				return
			}
		})
	})
}
