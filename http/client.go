// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package http

import (
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

type circuitOptions struct {
	name         string
	logger       *zap.Logger
	maxRequests  uint32
	interval     time.Duration
	timeout      time.Duration
	tripCount    uint32
	isSuccessful func(error) bool
	statusCodes  []int
}

type CircuitOption func(*circuitOptions)

// CircuitName is the name of the circuit breaker. This will be used to create a named logger
// for logging status changes.
func CircuitName(name string) CircuitOption {
	return func(co *circuitOptions) {
		co.name = name
	}
}

// CircuitLogger
func CircuitLogger(logger *zap.Logger) CircuitOption {
	return func(co *circuitOptions) {
		co.logger = logger
	}
}

// CircuitMaxRequests is the maximum number of requests allowed to pass through
// when the CircuitBreaker is half-open. If MaxRequests is 0, CircuitBreaker allows only 1 request.
func CircuitMaxRequests(maxRequests uint32) CircuitOption {
	return func(co *circuitOptions) {
		co.maxRequests = maxRequests
	}
}

// CircuitInterval is the cyclic period of the closed state for CircuitBreaker to
// clear the internal Counts, described later in this section. If Interval is 0,
// CircuitBreaker doesn't clear the internal Counts during the closed state.
//
// CircuitBreaker clears the internal Counts either on the change of the state or
// at the closed-state intervals. Counts ignores the results of the requests sent before clearing.
func CircuitInterval(interval time.Duration) CircuitOption {
	return func(co *circuitOptions) {
		co.interval = interval
	}
}

// CircuitTimeout is the period of the open state, after which the state of CircuitBreaker
// becomes half-open. If Timeout is 0, the timeout value of CircuitBreaker is set to 60 seconds.
func CircuitTimeout(timeout time.Duration) CircuitOption {
	return func(co *circuitOptions) {
		co.timeout = timeout
	}
}

// CircuitTripCount determines the number of consecutive failues required to trip the circuit.
func CircuitTripCount(n uint32) CircuitOption {
	return func(co *circuitOptions) {
		co.tripCount = n
	}
}

var errStatusCode = errors.New("status code error")

// CircuitErrorOnStatusCode allows you to register HTTP response status codes which
// should be counted as an error by the circuit breaker.
//
// Default: 400, 401, 403, 500
func CircuitErrorOnStatusCode(n int) CircuitOption {
	return func(co *circuitOptions) {
		co.statusCodes = append(co.statusCodes, n)
	}
}

// NotConnError
func NotConnError(err error) bool {
	e := errors.Unwrap(err)
	switch e.(type) {
	case *net.AddrError:
		return false
	case *net.DNSError:
		return false
	case *net.OpError:
		return false
	default:
		return true
	}
}

// NotStatusCodeError
func NotStatusCodeError(err error) bool {
	return err != errStatusCode
}

func composeCircuitErrorCheckers(fs ...func(error) bool) func(error) bool {
	return func(err error) bool {
		for _, f := range fs {
			ok := f(err)
			if ok {
				continue
			}
			return false
		}
		return true
	}
}

// CountCircuitErrorIf
func CountCircuitErrorIf(f func(error) bool) CircuitOption {
	return func(co *circuitOptions) {
		co.isSuccessful = f
	}
}

// RoundTripperOption
type RoundTripperOption func(http.RoundTripper) http.RoundTripper

// CircuitBreaker
func CircuitBreaker(opts ...CircuitOption) RoundTripperOption {
	return func(rt http.RoundTripper) http.RoundTripper {
		co := &circuitOptions{
			logger:      zap.NewNop(),
			tripCount:   5,
			timeout:     60 * time.Second,
			maxRequests: 1,
			isSuccessful: composeCircuitErrorCheckers(
				NotStatusCodeError,
				NotConnError,
			),
		}
		for _, opt := range opts {
			opt(co)
		}

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

		log := co.logger.Named(co.name)

		return &circuitRoundTripper{
			RoundTripper: rt,
			cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
				Name:        co.name,
				MaxRequests: co.maxRequests,
				Interval:    co.interval,
				Timeout:     co.timeout,
				ReadyToTrip: func(counts gobreaker.Counts) bool {
					return counts.ConsecutiveFailures >= co.tripCount
				},
				OnStateChange: func(name string, from, to gobreaker.State) {
					switch to {
					case gobreaker.StateOpen:
						log.Error("circuit has been opened")
					case gobreaker.StateHalfOpen:
						log.Warn("circuit is now half open and lettings some requests through", zap.Uint32("max_requests_allowed_through", co.maxRequests))
					case gobreaker.StateClosed:
						log.Info("circuit has been closed")
					}
				},
				IsSuccessful: co.isSuccessful,
			}),
			onStatusCode: func(n int) error {
				_, ok := codes[n]
				if !ok {
					return nil
				}
				return errStatusCode
			},
		}
	}
}

// RoundTripperWith
func RoundTripperWith(rt http.RoundTripper, opts ...RoundTripperOption) http.RoundTripper {
	for _, opt := range opts {
		rt = opt(rt)
	}
	return rt
}

type retryOptions struct {
	logger     *zap.Logger
	maxRetries int
	waitMin    time.Duration
	waitMax    time.Duration
}

type RetryOption func(*retryOptions)

func MinWaitDuration(min time.Duration) RetryOption {
	return func(ro *retryOptions) {
		ro.waitMin = min
	}
}

func MaxWaitDuration(max time.Duration) RetryOption {
	return func(ro *retryOptions) {
		ro.waitMax = max
	}
}

func MaxAttempts(maxAttempts int) RetryOption {
	return func(ro *retryOptions) {
		ro.maxRetries = maxAttempts
	}
}

func RetryAttemptLogger(logger *zap.Logger) RetryOption {
	return func(ro *retryOptions) {
		ro.logger = logger
	}
}

// RetryRequests configures adds request retry logic to an http.Client.
func RetryRequests(opts ...RetryOption) ClientOption {
	return func(co *clientOptions) {
		ro := &retryOptions{
			logger:     zap.NewNop(),
			waitMin:    100 * time.Millisecond,
			waitMax:    5 * time.Second,
			maxRetries: 2,
		}
		for _, opt := range opts {
			opt(ro)
		}
		co.retryOptions = ro
	}
}

type clientOptions struct {
	timeout      time.Duration
	transport    http.RoundTripper
	retryOptions *retryOptions
}

type ClientOption func(*clientOptions)

func ClientTimeout(timeout time.Duration) ClientOption {
	return func(co *clientOptions) {
		co.timeout = timeout
	}
}

func WithTransport(transport http.RoundTripper) ClientOption {
	return func(co *clientOptions) {
		co.transport = transport
	}
}

func NewClient(opts ...ClientOption) *http.Client {
	co := &clientOptions{
		transport: http.DefaultTransport,
	}
	for _, opt := range opts {
		opt(co)
	}
	c := &http.Client{
		Timeout:   co.timeout,
		Transport: co.transport,
	}
	if co.retryOptions == nil {
		return c
	}

	log := co.retryOptions.logger
	rc := retryablehttp.Client{
		HTTPClient:   c,
		Logger:       nil,
		RetryWaitMin: co.retryOptions.waitMin,
		RetryWaitMax: co.retryOptions.waitMax,
		RetryMax:     co.retryOptions.maxRetries,
		RequestLogHook: func(l retryablehttp.Logger, req *http.Request, i int) {
			log.Info("sending http request", zap.String("url", req.URL.String()), zap.Int("request_attempt_count", i))
		},
		ResponseLogHook: func(l retryablehttp.Logger, resp *http.Response) {
			log.Info("received http response", zap.String("url", resp.Request.URL.String()), zap.Int("http_status_code", resp.StatusCode))
		},
		CheckRetry:   retryablehttp.DefaultRetryPolicy,
		Backoff:      retryablehttp.DefaultBackoff,
		ErrorHandler: retryablehttp.PassthroughErrorHandler,
	}
	return rc.StandardClient()
}

type circuitRoundTripper struct {
	http.RoundTripper
	cb           *gobreaker.CircuitBreaker
	onStatusCode func(int) error
}

func (rt *circuitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	v, err := rt.cb.Execute(func() (interface{}, error) {
		resp, err := rt.RoundTripper.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		err = rt.onStatusCode(resp.StatusCode)
		if err != nil {
			return nil, err
		}
		return resp, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*http.Response), nil
}
