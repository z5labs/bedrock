// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package otlp provides bedrock Builders for creating OpenTelemetry Protocol (OTLP)
// exporters. It supports exporting traces, metrics, and logs over both gRPC and HTTP
// transports.
//
// Each exporter builder follows bedrock's functional composition pattern, accepting
// configuration readers and connection builders as inputs. The gRPC variants require
// a Builder for *grpc.ClientConn, while HTTP variants require an endpoint reader and
// an HTTP client builder.
package otlp
