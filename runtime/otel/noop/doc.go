// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package noop provides no-operation implementations of OpenTelemetry exporters
// for traces, metrics, and logs. These exporters discard all telemetry data,
// making them useful for testing, local development, or scenarios where
// telemetry export should be disabled without changing application code.
//
// Each exporter type implements the corresponding OpenTelemetry SDK exporter
// interface and comes with a builder function that follows bedrock's functional
// Builder pattern.
package noop
