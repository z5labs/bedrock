// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/swaggest/openapi-go/openapi3"
	"golang.org/x/sync/errgroup"
)

func TestApp(t *testing.T) {
	t.Run("will return OpenAPI spec", func(t *testing.T) {
		t.Run("if a request is sent to /openapi.json", func(t *testing.T) {
			addrCh := make(chan net.Addr)
			app := NewApp(
				func(a *App) {
					a.listen = func(network, addr string) (net.Listener, error) {
						ls, err := net.Listen(network, ":0")
						if err != nil {
							return nil, err
						}
						defer close(addrCh)

						addrCh <- ls.Addr()
						return ls, nil
					}
				},
			)

			ctx, cancel := context.WithCancel(context.Background())

			eg, egctx := errgroup.WithContext(ctx)
			eg.Go(func() error {
				return app.Run(egctx)
			})
			eg.Go(func() error {
				defer cancel()

				addr := <-addrCh
				resp, err := http.Get(fmt.Sprintf("http://%s/openapi.json", addr))
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				b, err := io.ReadAll(resp.Body)
				if err != nil {
					return err
				}

				var spec openapi3.Spec
				err = json.Unmarshal(b, &spec)
				if err != nil {
					return err
				}

				if spec.Openapi != "3.0.3" {
					return errors.New("incorrect version")
				}
				return nil
			})

			err := eg.Wait()
			if !assert.Nil(t, err) {
				return
			}
		})
	})
}

func TestApp_Run(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if it fails to marshal the OpenAPI spec to JSON", func(t *testing.T) {
			app := NewApp()

			marshalErr := errors.New("failed to marshal")
			app.marshalJSON = func(a any) ([]byte, error) {
				return nil, marshalErr
			}

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, marshalErr) {
				return
			}
		})

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
