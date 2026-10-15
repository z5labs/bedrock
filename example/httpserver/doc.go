// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Httpserver is a complete HTTP service built on bedrock with the standard
// library alone. It is the reference for building on bedrock, and it shows
// in plain Go what runtime/http did before it left bedrock: listening,
// server timeouts, configuration defaults and graceful shutdown.
//
// It serves GET /hello, which answers "hello, world".
//
// # Settings
//
// Every setting is read from the environment of the process. A setting
// that is not set takes its default. A setting that is set but malformed,
// such as READ_TIMEOUT=30 with no unit, fails startup with an error naming
// it on standard error; it never falls back to its default. Every malformed
// setting is reported at once.
//
//	variable               default   meaning
//	---------------------  --------  ------------------------------------------------
//	PORT                   8080      TCP port to listen on, 0..65535
//	READ_TIMEOUT           5s        http.Server.ReadTimeout
//	READ_HEADER_TIMEOUT    2s        http.Server.ReadHeaderTimeout
//	WRITE_TIMEOUT          10s       http.Server.WriteTimeout
//	IDLE_TIMEOUT           120s      http.Server.IdleTimeout
//	MAX_HEADER_BYTES       1048576   http.Server.MaxHeaderBytes
//	SHUTDOWN_GRACE_PERIOD  10s       how long in-flight requests get after a signal
//
// The durations are parsed with time.ParseDuration. The defaults of the
// timeouts and of MAX_HEADER_BYTES are those of runtime/http at v0.21.0.
//
// # Shutdown
//
// An interrupt or SIGTERM starts a graceful shutdown: the service stops
// accepting connections and waits for in-flight requests to finish, for at
// most SHUTDOWN_GRACE_PERIOD. It exits with status 0 if they finish in time,
// and with status 1 otherwise, after closing the connections still open.
//
// # Logging
//
// The service logs JSON to standard output with a logger of its own. It
// never replaces the default logger of log or log/slog.
package main
