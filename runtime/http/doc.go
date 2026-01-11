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
//   - TCPListener: Configures TCP network listeners using config.Reader
//   - Server: Configures HTTP server settings (timeouts, headers, etc.)
//   - Runtime: Implements bedrock.Runtime to run the HTTP server
//
// # Basic Usage
//
// Create an HTTP server by composing a listener, server configuration, and handler:
//
//	// Configure listener
//	listener := http.NewTCPListener(
//	    http.Addr(config.String(":8080")),
//	)
//
//	// Configure server with timeouts
//	server := http.NewServer(
//	    listener,
//	    http.ReadTimeout(config.Duration(5 * time.Second)),
//	    http.WriteTimeout(config.Duration(10 * time.Second)),
//	)
//
//	// Build runtime with handler
//	handlerBuilder := bedrock.BuilderFunc(func(ctx context.Context) (http.Handler, error) {
//	    return myHandler, nil
//	})
//
//	runtimeBuilder := http.Build(server, handlerBuilder)
//
//	// Run the server
//	runner := bedrock.DefaultRunner()
//	err := runner.Run(ctx, runtimeBuilder)
//
// # TLS Support
//
// Add TLS encryption by wrapping a listener:
//
//	baseLn := http.NewTCPListener(http.Addr(config.String(":8443")))
//	tlsConfig := config.ReaderFunc(func(ctx context.Context) (config.Value[*tls.Config], error) {
//	    // Load certificates and create tls.Config
//	    return config.ValueOf(cfg), nil
//	})
//	tlsLn := http.TLSListener(baseLn, tlsConfig)
//
//	server := http.NewServer(tlsLn)
//
// # Configuration
//
// All configuration uses bedrock's config.Reader pattern, allowing values to be
// sourced from environment variables, files, or composed from multiple sources:
//
//	// Read address from environment with fallback
//	addr := config.Or(
//	    config.Env("HTTP_ADDR"),
//	    config.String(":8080"),
//	)
//
//	listener := http.NewTCPListener(http.Addr(addr))
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
//	enhancedHandler := bedrock.Map(basicHandler, func(h http.Handler) (http.Handler, error) {
//	    return middleware.Wrap(h), nil
//	})
//
//	runtime := http.Build(server, enhancedHandler)
//
// # Default Values
//
// Server provides sensible defaults when options are not specified:
//
//   - DisableGeneralOptionsHandler: false
//   - ReadTimeout: 5 seconds
//   - ReadHeaderTimeout: 2 seconds
//   - WriteTimeout: 10 seconds
//   - IdleTimeout: 120 seconds
//   - MaxHeaderBytes: 1048576 bytes (1 MB)
//   - Listener address (TCPListener): ":8080"
package http
