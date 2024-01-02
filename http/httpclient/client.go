// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package httpclient provides a production ready http.Client.
package httpclient

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/z5labs/bedrock/pkg/noop"
	"github.com/z5labs/bedrock/pkg/slogfield"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/sony/gobreaker"
)

type circuitOptions struct {
	maxRequests  uint32
	interval     time.Duration
	timeout      time.Duration
	tripCount    uint32
	isSuccessful func(error) bool
	statusCodes  []int
}

func withCircuitOption(f func(*circuitOptions)) Option {
	return func(o *options) {
		if o.co == nil {
			o.co = new(circuitOptions)
		}
		f(o.co)
	}
}

func HalfOpenRequests(n uint32) Option {
	return withCircuitOption(func(co *circuitOptions) {
		co.maxRequests = n
	})
}

func OpenStateTimeout(d time.Duration) Option {
	return withCircuitOption(func(co *circuitOptions) {
		co.timeout = d
	})
}

func CountResetInterval(d time.Duration) Option {
	return withCircuitOption(func(co *circuitOptions) {
		co.interval = d
	})
}

func TripAfter(n uint32) Option {
	return withCircuitOption(func(co *circuitOptions) {
		co.tripCount = n
	})
}

func TripOn() Option {
	return withCircuitOption(func(co *circuitOptions) {

	})
}

type retryOptions struct {
	maxRetries int
	waitMin    time.Duration
	waitMax    time.Duration
}

type options struct {
	timeout time.Duration
	rt      http.RoundTripper

	name       string
	logHandler slog.Handler

	co *circuitOptions
	ro *retryOptions
}

type Option func(*options)

func Name(s string) Option {
	return func(o *options) {
		o.name = s
	}
}

func RoundTripper(rt http.RoundTripper) Option {
	return func(wo *options) {
		wo.rt = rt
	}
}

// Timeout provides a global timeout value for the http.Client.
func Timeout(d time.Duration) Option {
	return func(wo *options) {
		wo.timeout = d
	}
}

func LogHandler(h slog.Handler) Option {
	return func(wo *options) {
		wo.logHandler = h
	}
}

func New(opts ...Option) *http.Client {
	o := &options{
		rt:         http.DefaultTransport,
		logHandler: noop.LogHandler{},
	}
	for _, opt := range opts {
		opt(o)
	}

	logger := slog.New(o.logHandler)
	if o.name != "" {
		logger = logger.With(slogfield.String("http_client", o.name))
	}

	var rt http.RoundTripper = &logRoundTripper{
		base: o.rt,
		log:  logger,
	}

	if o.co != nil {
		co := o.co
		if len(co.statusCodes) == 0 {
			co.statusCodes = append(
				co.statusCodes,
				http.StatusBadRequest,          // 400
				http.StatusUnauthorized,        // 401
				http.StatusForbidden,           // 403
				http.StatusInternalServerError, // 500
			)
		}

		codes := map[int]struct{}{}
		for _, code := range co.statusCodes {
			codes[code] = struct{}{}
		}

		rt = &circuitRoundTripper{
			base: rt,
			cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
				Name:        o.name,
				MaxRequests: co.maxRequests,
				Interval:    co.interval,
				Timeout:     co.timeout,
				ReadyToTrip: func(counts gobreaker.Counts) bool {
					return counts.ConsecutiveFailures >= co.tripCount
				},
				OnStateChange: func(name string, from, to gobreaker.State) {
					switch to {
					case gobreaker.StateOpen:
						logger.Error("circuit has been opened")
					case gobreaker.StateHalfOpen:
						logger.Warn(
							"circuit is now half open and lettings some requests through",
							slogfield.Uint32("max_requests_allowed_through", co.maxRequests),
						)
					case gobreaker.StateClosed:
						logger.Info("circuit has been closed")
					}
				},
				IsSuccessful: co.isSuccessful,
			}),
			onStatusCode: func(n int) error {
				_, ok := codes[n]
				if !ok {
					return nil
				}
				return errors.New("status code error")
			},
		}
	}
	if o.ro == nil {
		return &http.Client{
			Timeout:   o.timeout,
			Transport: rt,
		}
	}

	ro := o.ro
	rc := retryablehttp.Client{
		HTTPClient: &http.Client{
			Timeout:   o.timeout,
			Transport: rt,
		},
		RetryWaitMin: ro.waitMin,
		RetryWaitMax: ro.waitMax,
		RetryMax:     ro.maxRetries,
		CheckRetry:   retryablehttp.DefaultRetryPolicy,
		Backoff:      retryablehttp.DefaultBackoff,
		ErrorHandler: retryablehttp.PassthroughErrorHandler,
	}
	return rc.StandardClient()
}

type logRoundTripper struct {
	base http.RoundTripper
	log  *slog.Logger
}

func (rt *logRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	start := time.Now()
	rt.log.InfoContext(
		ctx,
		"request sent",
		slogfield.String("url", req.URL.String()),
	)
	resp, err := rt.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	rt.log.InfoContext(
		ctx,
		"response received",
		slogfield.String("url", req.URL.String()),
		slogfield.Duration("latency", time.Since(start)),
	)
	return resp, err
}

type statusCodeError struct {
	code int
}

func (e statusCodeError) Error() string {
	return "error"
}

type circuitRoundTripper struct {
	base         http.RoundTripper
	cb           *gobreaker.CircuitBreaker
	onStatusCode func(int) error
}

func (rt *circuitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	v, err := rt.cb.Execute(func() (interface{}, error) {
		resp, err := rt.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		return resp, statusCodeError{code: resp.StatusCode}
	})
	if errors.Is(err, statusCodeError{}) {
		return v.(*http.Response), nil
	}
	if err != nil {
		return nil, err
	}
	return v.(*http.Response), nil
}
