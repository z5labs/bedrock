// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package http

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
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

func Example_tls() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Generate a self-signed certificate for TLS
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fmt.Println(err)
		return
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		fmt.Println(err)
		return
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}

	// Create a TCP listener first to get the address for testing
	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		fmt.Println(err)
		return
	}

	// Wrap the listener with TLS using BuildTLSListener
	// For real usage, you would use BuildTCPListener with a config.Reader[*net.TCPAddr]
	tlsListenerBuilder := BuildTLSListener(
		bedrock.BuilderOf(ls),
		config.ReaderOf(tlsConfig),
	)

	// Create an HTTP handler builder
	handlerBuilder := bedrock.BuilderFunc[http.Handler](func(ctx context.Context) (http.Handler, error) {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "Hello from bedrock with TLS!")
		})
		return mux, nil
	})

	// Build the runtime with server options
	runtimeBuilder := Build(
		tlsListenerBuilder,
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

	// Create HTTPS client that skips certificate verification for this example
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+ls.Addr().String(), nil)
	if err != nil {
		fmt.Println(err)
		return
	}

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
	// Hello from bedrock with TLS!
}
