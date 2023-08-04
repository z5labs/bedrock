// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package http

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

type clientOptions struct {
	logger *zap.Logger

	// base http client options
	baseHttpClient *http.Client

	// circuit breaker options
	name        string
	maxRequests uint32
	interval    time.Duration
	timeout     time.Duration

	// retry options
	maxRetries   int
	retryMinWait time.Duration
	retryMaxWait time.Duration
}

type ClientOption func(*clientOptions)

func Logger(logger *zap.Logger) ClientOption {
	return func(co *clientOptions) {
		co.logger = logger
	}
}

func BaseClient(hc *http.Client) ClientOption {
	return func(co *clientOptions) {
		co.baseHttpClient = hc
	}
}

type circuitBreakingRoundTripper struct {
	http.RoundTripper
	cb *gobreaker.CircuitBreaker
}

func (rt *circuitBreakingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	v, err := rt.cb.Execute(func() (interface{}, error) {
		return rt.RoundTripper.RoundTrip(req)
	})
	resp, _ := v.(*http.Response)
	return resp, err
}

type retryClient interface {
	Do(*retryablehttp.Request) (*http.Response, error)
}

// Client
type Client struct {
	http retryClient
}

// NewClient
func NewClient(opts ...ClientOption) *Client {
	copts := &clientOptions{
		logger:         zap.NewNop(),
		baseHttpClient: http.DefaultClient,
		maxRetries:     0,
		retryMinWait:   0,
		retryMaxWait:   0,
	}
	for _, opt := range opts {
		opt(copts)
	}

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:          copts.name,
		MaxRequests:   copts.maxRequests,
		Interval:      copts.interval,
		Timeout:       copts.timeout,
		ReadyToTrip:   configureReadyToTrip(copts),
		OnStateChange: nil, // TODO: prolly want to log when this happens
		IsSuccessful:  configureIsSuccessful(copts),
	})

	baseHttpClient := copts.baseHttpClient
	baseHttpClient.Transport = &circuitBreakingRoundTripper{
		RoundTripper: baseHttpClient.Transport,
		cb:           cb,
	}

	retryClient := &retryablehttp.Client{
		HTTPClient:      baseHttpClient,
		Logger:          nil,
		RetryWaitMin:    copts.retryMinWait,
		RetryWaitMax:    copts.retryMaxWait,
		RetryMax:        copts.maxRetries,
		RequestLogHook:  nil,
		ResponseLogHook: nil,
		CheckRetry:      retryablehttp.DefaultRetryPolicy,
		Backoff:         retryablehttp.DefaultBackoff,
	}
	return &Client{
		http: retryClient,
	}
}

func configureReadyToTrip(opts *clientOptions) func(counts gobreaker.Counts) bool {
	return func(counts gobreaker.Counts) bool {
		return counts.ConsecutiveFailures > 5
	}
}

func configureIsSuccessful(opts *clientOptions) func(err error) bool {
	return func(err error) bool {
		return err == nil
	}
}

// Do
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	// TODO: what attributes should be added to span
	spanCtx, span := otel.Tracer("http").Start(req.Context(), "Client.Do")
	defer span.End()

	req = req.WithContext(spanCtx)

	r, err := retryablehttp.FromRequest(req)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	resp, err := c.http.Do(r)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	return resp, nil
}

// Post
func (c *Client) Post(ctx context.Context, url string, body io.Reader) (*http.Response, error) {
	spanCtx, span := otel.Tracer("http").Start(ctx, "Client.Post")
	defer span.End()

	r, err := http.NewRequestWithContext(spanCtx, http.MethodPost, url, body)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	return c.Do(r)
}

// Get
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	spanCtx, span := otel.Tracer("http").Start(ctx, "Client.Get")
	defer span.End()

	r, err := http.NewRequestWithContext(spanCtx, http.MethodPost, url, nil)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	return c.Do(r)
}
