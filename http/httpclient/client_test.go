// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package httpclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/z5labs/bedrock/pkg/noop"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func serve(f func(http.ResponseWriter, *http.Request)) (addr string, stop func(), err error) {
	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", nil, err
	}
	addr = ls.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/", f)
	s := &http.Server{
		Handler: mux,
	}
	stop = func() {
		s.Shutdown(context.Background())
	}

	go func() {
		s.Serve(ls)
	}()
	return
}

func TestTimeout(t *testing.T) {
	t.Run("will timeout", func(t *testing.T) {
		t.Run("if the timeout is set to be greater than zero", func(t *testing.T) {
			timeout := 500 * time.Millisecond
			addr, stop, err := serve(func(w http.ResponseWriter, r *http.Request) {
				<-time.After(2 * timeout)
			})
			if !assert.Nil(t, err) {
				return
			}
			defer stop()

			client := New(Timeout(timeout))
			_, err = client.Get(fmt.Sprintf("http://%s/", addr))

			var nerr net.Error
			if !assert.ErrorAs(t, err, &nerr) {
				return
			}
			if !assert.True(t, nerr.Timeout()) {
				return
			}
		})
	})
}

func TestLogRoundTripper(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the base round tripper fails", func(t *testing.T) {
			rtErr := errors.New("round trip failed")
			base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return nil, rtErr
			})
			rt := &logRoundTripper{
				base: base,
				log:  slog.New(noop.LogHandler{}),
			}

			req, err := http.NewRequest(http.MethodGet, "http://example.org", nil)
			if !assert.Nil(t, err) {
				return
			}

			_, err = rt.RoundTrip(req)
			if !assert.Equal(t, rtErr, err) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if the base round tripper succeeds", func(t *testing.T) {
			base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return new(http.Response), nil
			})
			rt := &logRoundTripper{
				base: base,
				log:  slog.New(noop.LogHandler{}),
			}

			req, err := http.NewRequest(http.MethodGet, "http://example.org", nil)
			if !assert.Nil(t, err) {
				return
			}

			_, err = rt.RoundTrip(req)
			if !assert.Nil(t, err) {
				return
			}
		})
	})
}

func TestCircuitBreaker(t *testing.T) {
	t.Run("will not open the circuit", func(t *testing.T) {
		t.Run("if the counts have been reset after the configured interval", func(t *testing.T) {
			rtErr := errors.New("round trip failed")
			base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return nil, rtErr
			})
			c := New(
				RoundTripper(base),
				TripAfter(3),
				CountResetInterval(1*time.Second),
				TripOn(func(r *http.Response, err error) bool {
					return err != nil
				}),
			)

			var err error
			for i := 0; i < 2; i++ {
				_, err = c.Get("http://example.org")
				if !assert.ErrorIs(t, err, rtErr) {
					return
				}
			}

			<-time.After(1200 * time.Millisecond)
			_, err = c.Get("http://example.org")
			if !assert.NotErrorIs(t, err, gobreaker.ErrOpenState) {
				return
			}
		})
	})

	t.Run("will open the circuit", func(t *testing.T) {
		t.Run("if consecutive failues is greater than or equal to configured trip count", func(t *testing.T) {
			rtErr := errors.New("round trip failed")
			base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return nil, rtErr
			})
			c := New(
				RoundTripper(base),
				TripAfter(3),
				TripOn(func(r *http.Response, err error) bool {
					return err != nil
				}),
			)

			var err error
			for i := 0; i < 4; i++ {
				_, err = c.Get("http://example.org")
			}
			if !assert.ErrorIs(t, err, gobreaker.ErrOpenState) {
				return
			}
		})
	})

	t.Run("will close the circuit", func(t *testing.T) {
		t.Run("if consecutive successes is greater than or equal to max requests after the provided timeout duration", func(t *testing.T) {
			halfOpen := false
			base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				if !halfOpen {
					return nil, errors.New("failure")
				}
				return new(http.Response), nil
			})
			c := New(
				RoundTripper(base),
				TripAfter(1),
				HalfOpenRequests(2),
				OpenStateTimeout(1*time.Second),
				TripOn(func(r *http.Response, err error) bool {
					return err != nil
				}),
			)

			var err error
			for i := 0; i < 3; i++ {
				_, err = c.Get("http://example.org")
			}
			if !assert.ErrorIs(t, err, gobreaker.ErrOpenState) {
				return
			}

			<-time.After(1200 * time.Millisecond)
			halfOpen = true
			for i := 0; i < 3; i++ {
				_, err = c.Get("http://example.org")
				if !assert.Nil(t, err) {
					return
				}
			}
		})
	})
}

func TestRetry(t *testing.T) {
	t.Run("will retry the request", func(t *testing.T) {
		t.Run("if the provided retry on function returns true", func(t *testing.T) {
			var callCount int
			base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				callCount += 1
				return new(http.Response), nil
			})
			c := New(
				RoundTripper(base),
				MaxRetries(1),
				MinRetryAfter(100*time.Millisecond),
				MaxRetryAfter(1*time.Second),
				RetryOn(func(r *http.Response, err error) bool {
					return true
				}),
			)

			resp, err := c.Get("http://example.org")
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Zero(t, resp.StatusCode) {
				return
			}
			if !assert.Equal(t, 2, callCount) {
				return
			}
		})
	})

	t.Run("will not retry the request", func(t *testing.T) {
		t.Run("if the provided retry on function returns false", func(t *testing.T) {
			var callCount int
			base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				callCount += 1
				return new(http.Response), nil
			})
			c := New(
				RoundTripper(base),
				MaxRetries(1),
				MinRetryAfter(100*time.Millisecond),
				MaxRetryAfter(1*time.Second),
				RetryOn(func(r *http.Response, err error) bool {
					return false
				}),
			)

			resp, err := c.Get("http://example.org")
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Zero(t, resp.StatusCode) {
				return
			}
			if !assert.Equal(t, 1, callCount) {
				return
			}
		})
	})
}
