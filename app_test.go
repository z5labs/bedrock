// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/z5labs/bedrock/pkg/config"
)

type configSourceFunc func(config.Store) error

func (f configSourceFunc) Apply(store config.Store) error {
	return f(store)
}

var unmarshalErr = errors.New("failed to unmarshal")

type UnmarshalTextFailure struct{}

func (x UnmarshalTextFailure) UnmarshalText(b []byte) error {
	return unmarshalErr
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

		t.Run("if the config.Manager fails to unmarshal the custom config", func(t *testing.T) {
			type myConfig struct {
				Value UnmarshalTextFailure `config:"value"`
			}
			b := AppBuilderFunc[myConfig](func(ctx context.Context, cfg myConfig) (App, error) {
				return nil, nil
			})

			err := Run(context.Background(), b, config.FromYaml(strings.NewReader(`value: hello`)))

			var ierr ConfigUnmarshalError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
			if !assert.ErrorIs(t, ierr, unmarshalErr) {
				return
			}
		})

		t.Run("if the AppBuilder fails to build the App", func(t *testing.T) {
			type myConfig struct {
				Value string `config:"value"`
			}

			buildErr := errors.New("failed to build")
			b := AppBuilderFunc[myConfig](func(ctx context.Context, cfg myConfig) (App, error) {
				return nil, buildErr
			})

			err := Run(context.Background(), b, config.FromYaml(strings.NewReader(`value: hello`)))

			var ierr AppBuildError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
			if !assert.ErrorIs(t, ierr, buildErr) {
				return
			}
		})

		t.Run("if the App fails to run", func(t *testing.T) {
			type myConfig struct {
				Value string `config:"value"`
			}

			runErr := errors.New("failed to build")
			b := AppBuilderFunc[myConfig](func(ctx context.Context, cfg myConfig) (App, error) {
				app := appFunc(func(ctx context.Context) error {
					return runErr
				})
				return app, nil
			})

			err := Run(context.Background(), b, config.FromYaml(strings.NewReader(`value: hello`)))

			var ierr AppRunError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
			if !assert.ErrorIs(t, ierr, runErr) {
				return
			}
		})
	})
}
