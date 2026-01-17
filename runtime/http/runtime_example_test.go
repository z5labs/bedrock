// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package http

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"
)

func Example() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		fmt.Println(err)
		return
	}

	// Create a TCP listener builder with dynamic port for testing
	// For real usage, use BuildTCPListener with a proper config.Reader[*net.TCPAddr]
	listenerBuilder := bedrock.BuilderOf(ls)

	// Create an HTTP handler builder
	handlerBuilder := bedrock.BuilderFunc[http.Handler](func(ctx context.Context) (http.Handler, error) {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "Hello from bedrock!")
		})
		return mux, nil
	})

	// Build the runtime with custom timeouts
	runtimeBuilder := Build(
		listenerBuilder,
		handlerBuilder,
		DisableGeneralOptionsHandler(config.ReaderOf(false)),
		ReadTimeout(config.ReaderOf(5*time.Second)),
		ReadHeaderTimeout(config.ReaderOf(2*time.Second)),
		WriteTimeout(config.ReaderOf(10*time.Second)),
		IdleTimeout(config.ReaderOf(120*time.Second)),
		MaxHeaderBytes(config.ReaderOf(1048576)),
	)

	// Create a default runner
	runner := bedrock.DefaultRunner[Runtime]()

	// Run the server
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		err := runner.Run(ctx, runtimeBuilder)
		errCh <- err
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ls.Addr().String(), nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()

	_, err = io.Copy(os.Stdout, resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Output:
	// Hello from bedrock!
}
