// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package stdout provides bedrock builders for OpenTelemetry exporters that
// write telemetry data to an io.Writer.
//
// This package wraps the OpenTelemetry stdout exporters for traces, metrics,
// and logs, adapting them to the bedrock Builder pattern. Each builder accepts
// a Builder[io.Writer] allowing flexible configuration of the output destination.
//
// The package provides three builder functions:
//   - BuildSpanExporter: Creates a span exporter for tracing data
//   - BuildMetricExporter: Creates a metric exporter for metrics data
//   - BuildLogExporter: Creates a log exporter for log records
//
// These exporters are useful for local development, debugging, and testing
// where telemetry output to stdout or a file is desired instead of sending
// to a remote collector.
package stdout
