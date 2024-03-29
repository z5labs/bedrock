// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package httpclient

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/z5labs/bedrock/pkg/noop"
	"github.com/z5labs/bedrock/pkg/slogfield"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
)

type circuitOptions struct {
	maxRequests uint32
	interval    time.Duration
	timeout     time.Duration
	tripCount   uint32
	trippers    []func(*http.Response, error) bool
}

// CircuitOption are Options specifically for configuring the request circuit breaker.
type CircuitOption interface {
	Option

	setCircuitOption(*circuitOptions)
}

type circuitOptionFunc func(*circuitOptions)

func (f circuitOptionFunc) setCircuitOption(co *circuitOptions) {
	f(co)
}

func (f circuitOptionFunc) setOption(o *options) {
	if o.co == nil {
		o.co = &circuitOptions{}
	}
	f.setCircuitOption(o.co)
}

// HalfOpenRequests is the maximum number of requests allowed to pass
// through when the CircuitBreaker is half-open. If HalfOpenRequests is 0,
// the CircuitBreaker allows only 1 request.
func HalfOpenRequests(n uint32) CircuitOption {
	return circuitOptionFunc(func(co *circuitOptions) {
		co.maxRequests = n
	})
}

// OpenStateTimeout is the period of the open state, after which the
// state of the CircuitBreaker becomes half-open. The default timeout
// is 60 seconds.
func OpenStateTimeout(d time.Duration) CircuitOption {
	return circuitOptionFunc(func(co *circuitOptions) {
		co.timeout = d
	})
}

// CountResetInterval is the cyclic period of the closed state for the
// CircuitBreaker to clear the internal Counts. If CountResetInterval is less than
// or equal to 0, the CircuitBreaker doesn't clear internal Counts during
// the closed state.
func CountResetInterval(d time.Duration) CircuitOption {
	return circuitOptionFunc(func(co *circuitOptions) {
		co.interval = d
	})
}

// TripAfter will cause the circuit to become open if the number
// of consecutive failuires is greater than or equal to n.
func TripAfter(n uint32) CircuitOption {
	return circuitOptionFunc(func(co *circuitOptions) {
		co.tripCount = n
	})
}

// TripOn registers functions for determining is the circuit should
// be opened based on the http.Response and error returned by the
// underlying http.RoundTripper.
func TripOn(trippers ...func(*http.Response, error) bool) CircuitOption {
	return circuitOptionFunc(func(co *circuitOptions) {
		co.trippers = trippers
	})
}

type retryOptions struct {
	maxRetries int
	waitMin    time.Duration
	waitMax    time.Duration
	retryers   []func(*http.Response, error) bool
}

// RetryOption are Options specifically for configuring request retry attempts.
type RetryOption interface {
	Option

	setRetryOption(*retryOptions)
}

type retryOptionFunc func(*retryOptions)

func (f retryOptionFunc) setRetryOption(ro *retryOptions) {
	f(ro)
}

func (f retryOptionFunc) setOption(o *options) {
	if o.ro == nil {
		o.ro = &retryOptions{
			maxRetries: 2,
			waitMin:    200 * time.Millisecond,
			waitMax:    1 * time.Second,
		}
	}
	f.setRetryOption(o.ro)
}

// MaxRetries specifies the maximum number of retries attempted.
func MaxRetries(n int) RetryOption {
	return retryOptionFunc(func(ro *retryOptions) {
		ro.maxRetries = n
	})
}

// MinRetryAfter specifies the minimum time to wait before retrying a request.
func MinRetryAfter(d time.Duration) RetryOption {
	return retryOptionFunc(func(ro *retryOptions) {
		ro.waitMin = d
	})
}

// MaxRetryAfter specifies the maximum time to wait before retrying a request.
func MaxRetryAfter(d time.Duration) RetryOption {
	return retryOptionFunc(func(ro *retryOptions) {
		ro.waitMax = d
	})
}

// RetryOn specifies the conditions for whether a request should be retried or not.
func RetryOn(fs ...func(*http.Response, error) bool) RetryOption {
	return retryOptionFunc(func(ro *retryOptions) {
		ro.retryers = fs
	})
}

type oauthOptions struct {
	tokSrc oauth2.TokenSource
}

// OAuthOption are Options specifically for configuring OAuth.
type OAuthOption interface {
	Option

	setOAuthOption(*oauthOptions)
}

type oauthOptionFunc func(*oauthOptions)

func (f oauthOptionFunc) setOAuthOption(oo *oauthOptions) {
	f(oo)
}

func (f oauthOptionFunc) setOption(o *options) {
	if o.oauthOptions == nil {
		o.oauthOptions = &oauthOptions{}
	}
	f.setOAuthOption(o.oauthOptions)
}

// OAuth enables automatically adding the Authorization HTTP header
// with its value being an OAuth Bearer token.
func OAuth(ts oauth2.TokenSource) OAuthOption {
	return oauthOptionFunc(func(oo *oauthOptions) {
		oo.tokSrc = ts
	})
}

type otelOptions struct {
	opts []otelhttp.Option
}

// OTelOption are Options specifically for configuring OpenTelemetry.
type OTelOption interface {
	Option

	setOtelConfig(*otelOptions)
}

type otelOptionFunc func(*otelOptions)

func (f otelOptionFunc) setOtelConfig(oo *otelOptions) {
	f(oo)
}

func (f otelOptionFunc) setOption(o *options) {
	if o.otelOptions == nil {
		o.otelOptions = &otelOptions{}
	}
	f.setOtelConfig(o.otelOptions)
}

// OTel enables wrapping outbound requests with a span.
func OTel(opts ...otelhttp.Option) OTelOption {
	return otelOptionFunc(func(oo *otelOptions) {
		oo.opts = opts
	})
}

type options struct {
	timeout time.Duration
	rt      http.RoundTripper

	name       string
	logHandler slog.Handler

	co           *circuitOptions
	ro           *retryOptions
	oauthOptions *oauthOptions
	otelOptions  *otelOptions
}

// Option is used to configure a http.Client in a functional manner.
type Option interface {
	setOption(*options)
}

type optionFunc func(*options)

func (f optionFunc) setOption(o *options) {
	f(o)
}

// Name allows for naming this clients circuit breaker and providing a field
// in any logs where the key is "http_client" and the value being
// the name provided to this option.
func Name(s string) Option {
	return optionFunc(func(o *options) {
		o.name = s
	})
}

// RoundTripper allows you to provide a custom base http.RoundTripper which
// all other capabilities, such as, circuit breaking and retries will wrap around.
func RoundTripper(rt http.RoundTripper) Option {
	return optionFunc(func(wo *options) {
		wo.rt = rt
	})
}

// Timeout provides a global timeout value for the http.Client.
func Timeout(d time.Duration) Option {
	return optionFunc(func(wo *options) {
		wo.timeout = d
	})
}

// LogHandler enables the http.Client to provide logs around:
//   - sending requests
//   - receiving responses
//   - circuit state changes
//   - retry attempts
func LogHandler(h slog.Handler) Option {
	return optionFunc(func(wo *options) {
		wo.logHandler = h
	})
}

type initState struct {
	rt     http.RoundTripper
	logger *slog.Logger
}

// New helps you construct a production-ready http.Client using functional options.
func New(opts ...Option) *http.Client {
	o := &options{
		rt: http.DefaultTransport,
	}
	for _, opt := range opts {
		opt.setOption(o)
	}

	state := &initState{
		rt:     o.rt,
		logger: slog.New(noop.LogHandler{}),
	}

	// This list will wrap the starting RoundTripper one after another.
	// Thus, the order of this slice must be maintained for certain
	// initializations. Please document any specific ordering within
	// the slice itself.
	initers := []func(*options, *initState){
		withOTel,
		withOAuth,
		withLogging,
		// always put retry after circuit breaker so
		// retried requests go through the circuit breaker
		withCircuitBreaker,
		withRetries,
	}
	for _, initer := range initers {
		initer(o, state)
	}

	return &http.Client{
		Timeout:   o.timeout,
		Transport: state.rt,
	}
}

func withOTel(opts *options, state *initState) {
	if opts.otelOptions == nil {
		return
	}

	state.rt = otelhttp.NewTransport(state.rt, opts.otelOptions.opts...)
}

func withOAuth(opts *options, state *initState) {
	if opts.oauthOptions == nil {
		return
	}

	state.rt = &oauth2.Transport{
		Source: opts.oauthOptions.tokSrc,
		Base:   state.rt,
	}
}

func withLogging(opts *options, state *initState) {
	if opts.logHandler == nil {
		return
	}

	state.logger = slog.New(opts.logHandler)
	if opts.name != "" {
		state.logger = state.logger.With(slogfield.String("http_client", opts.name))
	}

	state.rt = &logRoundTripper{
		base: state.rt,
		log:  state.logger,
	}
}

func withCircuitBreaker(opts *options, state *initState) {
	if opts.co == nil {
		return
	}
	co := opts.co

	logger := state.logger
	state.rt = &circuitRoundTripper{
		base: state.rt,
		cb: gobreaker.NewTwoStepCircuitBreaker(gobreaker.Settings{
			Name:        opts.name,
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
		}),
		trippers: co.trippers,
	}
}

func withRetries(opts *options, state *initState) {
	if opts.ro == nil {
		return
	}

	ro := opts.ro
	rc := retryablehttp.Client{
		HTTPClient: &http.Client{
			Transport: state.rt,
		},
		RetryWaitMin: ro.waitMin,
		RetryWaitMax: ro.waitMax,
		RetryMax:     ro.maxRetries,
		CheckRetry: func(ctx context.Context, resp *http.Response, err error) (bool, error) {
			for _, retryOn := range ro.retryers {
				if retryOn(resp, err) {
					return true, nil
				}
			}
			return false, nil
		},
		Backoff:      retryablehttp.LinearJitterBackoff,
		ErrorHandler: retryablehttp.PassthroughErrorHandler,
	}
	state.rt = &retryablehttp.RoundTripper{
		Client: &rc,
	}
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
	return resp, nil
}

type circuitRoundTripper struct {
	base     http.RoundTripper
	cb       *gobreaker.TwoStepCircuitBreaker
	trippers []func(*http.Response, error) bool
}

func (rt *circuitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	done, err := rt.cb.Allow()
	if err != nil {
		return nil, err
	}
	resp, err := rt.base.RoundTrip(req)
	for _, tripOn := range rt.trippers {
		if tripOn(resp, err) {
			done(false)
			return resp, err
		}
	}
	done(true)
	return resp, err
}
