// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package http provides HTTP server runtime functionality for bedrock applications.
//
// This package implements bedrock's Runtime interface to run HTTP servers with
// support for graceful shutdown, configurable timeouts, TLS, and functional composition.
//
// # Core Components
//
// The package provides three main building blocks:
//
//   - BuildTCPListener: Creates a bedrock.Builder for TCP network listeners
//   - BuildTLSListener: Creates a bedrock.Builder that wraps a listener with TLS
//   - Build: Creates a bedrock.Builder that constructs an HTTP server Runtime
//
// # Basic Usage
//
// Create an HTTP server by composing a listener builder, handler builder, and server options:
//
//	// Build TCP listener
//	addrReader := config.ReaderFunc(func(ctx context.Context) (config.Value[*net.TCPAddr], error) {
//	    addr, err := net.ResolveTCPAddr("tcp", ":8080")
//	    return config.ValueOf(addr), err
//	})
//	listenerBuilder := http.BuildTCPListener(addrReader)
//
//	// Build handler
//	handlerBuilder := bedrock.BuilderFunc(func(ctx context.Context) (http.Handler, error) {
//	    return myHandler, nil
//	})
//
//	// Build runtime with server options
//	runtimeBuilder := http.Build(
//	    listenerBuilder,
//	    handlerBuilder,
//	    http.ReadTimeout(config.ReaderFunc(func(ctx context.Context) (config.Value[time.Duration], error) {
//	        return config.ValueOf(5 * time.Second), nil
//	    })),
//	    http.WriteTimeout(config.ReaderFunc(func(ctx context.Context) (config.Value[time.Duration], error) {
//	        return config.ValueOf(10 * time.Second), nil
//	    })),
//	)
//
//	// Run the server
//	runner := bedrock.DefaultRunner()
//	err := runner.Run(ctx, runtimeBuilder)
//
// # TLS Support
//
// Add TLS encryption by wrapping a listener builder with BuildTLSListener:
//
//	baseLnBuilder := http.BuildTCPListener(addrReader)
//	tlsConfigReader := config.ReaderFunc(func(ctx context.Context) (config.Value[*tls.Config], error) {
//	    // Load certificates and create tls.Config
//	    return config.ValueOf(cfg), nil
//	})
//	tlsLnBuilder := http.BuildTLSListener(baseLnBuilder, tlsConfigReader)
//
//	runtimeBuilder := http.Build(tlsLnBuilder, handlerBuilder)
//
// # Configuration
//
// All configuration uses bedrock's config.Reader pattern, allowing values to be
// sourced from environment variables, files, or composed from multiple sources:
//
//	// Read timeout from environment with fallback
//	timeout := config.Or(
//	    config.Map(config.Env("READ_TIMEOUT"), time.ParseDuration),
//	    config.ReaderFunc(func(ctx context.Context) (config.Value[time.Duration], error) {
//	        return config.ValueOf(5 * time.Second), nil
//	    }),
//	)
//
//	runtimeBuilder := http.Build(
//	    listenerBuilder,
//	    handlerBuilder,
//	    http.ReadTimeout(timeout),
//	)
//
// # Graceful Shutdown
//
// The Runtime automatically handles graceful shutdown when the context is cancelled.
// Combine with bedrock.NotifyOnSignal to handle OS signals:
//
//	runner := bedrock.NotifyOnSignal(
//	    bedrock.DefaultRunner(),
//	    syscall.SIGINT,
//	    syscall.SIGTERM,
//	)
//	err := runner.Run(ctx, runtimeBuilder)
//
// # Functional Composition
//
// The package follows bedrock's functional patterns. Use bedrock.Map and bedrock.Bind
// to compose builders:
//
//	// Transform a handler builder
//	enhancedHandler := bedrock.Map(basicHandler, func(ctx context.Context, h http.Handler) (http.Handler, error) {
//	    return middleware.Wrap(h), nil
//	})
//
//	runtimeBuilder := http.Build(listenerBuilder, enhancedHandler)
//
// # Server Options
//
// The Build function accepts ServerOption functions to configure the HTTP server:
//
//   - DisableGeneralOptionsHandler(config.Reader[bool]): Controls automatic OPTIONS handling
//   - ReadTimeout(config.Reader[time.Duration]): Maximum duration for reading entire request
//   - ReadHeaderTimeout(config.Reader[time.Duration]): Maximum duration for reading headers
//   - WriteTimeout(config.Reader[time.Duration]): Maximum duration before timing out writes
//   - IdleTimeout(config.Reader[time.Duration]): Maximum duration to wait for next request with keep-alives
//   - MaxHeaderBytes(config.Reader[int]): Maximum bytes for request header parsing
//
// # Default Values
//
// When server options are not specified, the following defaults are applied:
//
//   - DisableGeneralOptionsHandler: false
//   - ReadTimeout: 5 seconds
//   - ReadHeaderTimeout: 2 seconds
//   - WriteTimeout: 10 seconds
//   - IdleTimeout: 120 seconds
//   - MaxHeaderBytes: 1048576 bytes (1 MB)
package http
