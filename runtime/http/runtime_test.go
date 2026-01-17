// Copyright (c) 2023 Z5Labs and Contributors
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
	"math/big"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"

	"github.com/stretchr/testify/require"
)

// createTestTLSConfig dynamically generates a self-signed TLS config for testing
func createTestTLSConfig(t *testing.T) *tls.Config {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Create certificate template
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
	}

	// Create self-signed certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	// Create TLS certificate
	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}
}

func TestBuildTCPListener(t *testing.T) {
	testCases := []struct {
		name       string
		addr       config.Reader[*net.TCPAddr]
		expectAddr string // partial match for address (e.g., ":8080", ":9000")
		expectErr  bool
	}{
		{
			name:       "creates listener with default address (port 0)",
			addr:       config.ReaderOf(&net.TCPAddr{Port: 0}),
			expectAddr: ":", // dynamic port
		},
		{
			name:       "creates listener with custom port",
			addr:       config.ReaderOf(&net.TCPAddr{Port: 9000}),
			expectAddr: ":9000",
		},
		{
			name:       "creates listener with dynamic port explicitly",
			addr:       config.ReaderOf(&net.TCPAddr{Port: 0}),
			expectAddr: ":", // any port is fine
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			builder := BuildTCPListener(tc.addr)

			ctx := context.Background()
			ln, err := builder.Build(ctx)

			if tc.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, ln)
			defer ln.Close()

			addr := ln.Addr().String()
			require.Contains(t, addr, tc.expectAddr)

			// For dynamic port (:0), verify we got a non-zero port
			if tc.expectAddr == ":" {
				require.NotEqual(t, ":0", addr, "should have allocated a dynamic port")
			}
		})
	}
}

func TestBuild(t *testing.T) {
	testCases := []struct {
		name         string
		listener     bedrock.Builder[net.Listener]
		handlerFunc  func() bedrock.Builder[http.Handler]
		options      []ServerOption
		expectErr    bool
		verifyServer func(t *testing.T, rt Runtime)
	}{
		{
			name: "builds runtime with handler",
			listener: bedrock.Map(BuildTCPListener(config.ReaderOf(&net.TCPAddr{Port: 0})), func(ctx context.Context, ln *net.TCPListener) (net.Listener, error) {
				return ln, nil
			}),
			handlerFunc: func() bedrock.Builder[http.Handler] {
				return bedrock.BuilderFunc[http.Handler](func(ctx context.Context) (http.Handler, error) {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
					}), nil
				})
			},
			options: []ServerOption{
				DisableGeneralOptionsHandler(config.ReaderOf(false)),
				ReadTimeout(config.ReaderOf(5 * time.Second)),
				ReadHeaderTimeout(config.ReaderOf(2 * time.Second)),
				WriteTimeout(config.ReaderOf(10 * time.Second)),
				IdleTimeout(config.ReaderOf(120 * time.Second)),
				MaxHeaderBytes(config.ReaderOf(1048576)),
			},
			verifyServer: func(t *testing.T, rt Runtime) {
				require.NotNil(t, rt.ls)
				require.NotNil(t, rt.srv)
				require.NotNil(t, rt.srv.Handler)

				// Verify default timeout values
				require.Equal(t, 5*time.Second, rt.srv.ReadTimeout)
				require.Equal(t, 2*time.Second, rt.srv.ReadHeaderTimeout)
				require.Equal(t, 10*time.Second, rt.srv.WriteTimeout)
				require.Equal(t, 120*time.Second, rt.srv.IdleTimeout)

				// Verify default MaxHeaderBytes
				require.Equal(t, 1048576, rt.srv.MaxHeaderBytes)

				// Verify default DisableGeneralOptionsHandler
				require.False(t, rt.srv.DisableGeneralOptionsHandler)

				// Close listener
				rt.ls.Close()
			},
		},
		{
			name: "applies custom timeout values",
			listener: bedrock.Map(BuildTCPListener(config.ReaderOf(&net.TCPAddr{Port: 0})), func(ctx context.Context, ln *net.TCPListener) (net.Listener, error) {
				return ln, nil
			}),
			options: []ServerOption{
				DisableGeneralOptionsHandler(config.ReaderOf(false)),
				ReadTimeout(config.ReaderOf(20 * time.Second)),
				ReadHeaderTimeout(config.ReaderOf(5 * time.Second)),
				WriteTimeout(config.ReaderOf(30 * time.Second)),
				IdleTimeout(config.ReaderOf(180 * time.Second)),
				MaxHeaderBytes(config.ReaderOf(1048576)),
			},
			handlerFunc: func() bedrock.Builder[http.Handler] {
				return bedrock.BuilderFunc[http.Handler](func(ctx context.Context) (http.Handler, error) {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil
				})
			},
			verifyServer: func(t *testing.T, rt Runtime) {
				require.Equal(t, 20*time.Second, rt.srv.ReadTimeout)
				require.Equal(t, 5*time.Second, rt.srv.ReadHeaderTimeout)
				require.Equal(t, 30*time.Second, rt.srv.WriteTimeout)
				require.Equal(t, 180*time.Second, rt.srv.IdleTimeout)
				rt.ls.Close()
			},
		},
		{
			name: "applies custom MaxHeaderBytes",
			listener: bedrock.Map(BuildTCPListener(config.ReaderOf(&net.TCPAddr{Port: 0})), func(ctx context.Context, ln *net.TCPListener) (net.Listener, error) {
				return ln, nil
			}),
			options: []ServerOption{
				DisableGeneralOptionsHandler(config.ReaderOf(false)),
				ReadTimeout(config.ReaderOf(5 * time.Second)),
				ReadHeaderTimeout(config.ReaderOf(2 * time.Second)),
				WriteTimeout(config.ReaderOf(10 * time.Second)),
				IdleTimeout(config.ReaderOf(120 * time.Second)),
				MaxHeaderBytes(config.ReaderOf(2097152)),
			},
			handlerFunc: func() bedrock.Builder[http.Handler] {
				return bedrock.BuilderFunc[http.Handler](func(ctx context.Context) (http.Handler, error) {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil
				})
			},
			verifyServer: func(t *testing.T, rt Runtime) {
				require.Equal(t, 2097152, rt.srv.MaxHeaderBytes)
				rt.ls.Close()
			},
		},
		{
			name: "applies custom DisableGeneralOptionsHandler",
			listener: bedrock.Map(BuildTCPListener(config.ReaderOf(&net.TCPAddr{Port: 0})), func(ctx context.Context, ln *net.TCPListener) (net.Listener, error) {
				return ln, nil
			}),
			options: []ServerOption{
				DisableGeneralOptionsHandler(config.ReaderOf(true)),
				ReadTimeout(config.ReaderOf(5 * time.Second)),
				ReadHeaderTimeout(config.ReaderOf(2 * time.Second)),
				WriteTimeout(config.ReaderOf(10 * time.Second)),
				IdleTimeout(config.ReaderOf(120 * time.Second)),
				MaxHeaderBytes(config.ReaderOf(1048576)),
			},
			handlerFunc: func() bedrock.Builder[http.Handler] {
				return bedrock.BuilderFunc[http.Handler](func(ctx context.Context) (http.Handler, error) {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil
				})
			},
			verifyServer: func(t *testing.T, rt Runtime) {
				require.True(t, rt.srv.DisableGeneralOptionsHandler)
				rt.ls.Close()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runtimeBuilder := Build(tc.listener, tc.handlerFunc(), tc.options...)

			ctx := context.Background()
			rt, err := runtimeBuilder.Build(ctx)

			if tc.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tc.verifyServer != nil {
				tc.verifyServer(t, rt)
			}
		})
	}
}

func TestBuildTLSListener(t *testing.T) {
	t.Run("wraps listener with TLS config", func(t *testing.T) {
		baseListener := BuildTCPListener(config.ReaderOf(&net.TCPAddr{Port: 0}))
		tlsConfig := config.ReaderOf(createTestTLSConfig(t))

		tlsListenerBuilder := BuildTLSListener(baseListener, tlsConfig)

		ctx := context.Background()
		ln, err := tlsListenerBuilder.Build(ctx)
		require.NoError(t, err)

		require.NotNil(t, ln)
		require.NotNil(t, ln.Addr())

		ln.Close()
	})
}

func TestRuntime_Run(t *testing.T) {
	t.Run("serves HTTP requests", func(t *testing.T) {
		// Create listener builder
		listenerBuilder := bedrock.Map(BuildTCPListener(config.ReaderOf(&net.TCPAddr{Port: 0})), func(ctx context.Context, ln *net.TCPListener) (net.Listener, error) {
			return ln, nil
		})

		// Create handler builder
		handlerBuilder := bedrock.BuilderFunc[http.Handler](func(ctx context.Context) (http.Handler, error) {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}), nil
		})

		// Build runtime using Build() function
		runtimeBuilder := Build(listenerBuilder, handlerBuilder,
			DisableGeneralOptionsHandler(config.ReaderOf(false)),
			ReadTimeout(config.ReaderOf(5*time.Second)),
			ReadHeaderTimeout(config.ReaderOf(2*time.Second)),
			WriteTimeout(config.ReaderOf(10*time.Second)),
			IdleTimeout(config.ReaderOf(120*time.Second)),
			MaxHeaderBytes(config.ReaderOf(1048576)),
		)

		// Build the runtime
		ctx := context.Background()
		runtime, err := runtimeBuilder.Build(ctx)
		require.NoError(t, err)

		// Get listener address for requests
		addr := runtime.ls.Addr().String()

		// Run on separate goroutine
		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- runtime.Run(runCtx)
		}()

		// Wait for server ready
		time.Sleep(100 * time.Millisecond)

		// Make real HTTP request
		resp, err := http.Get("http://" + addr)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()

		// Trigger shutdown
		cancel()

		// Verify clean shutdown
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for shutdown")
		}
	})

	t.Run("handles multiple concurrent requests", func(t *testing.T) {
		listenerBuilder := bedrock.Map(BuildTCPListener(config.ReaderOf(&net.TCPAddr{Port: 0})), func(ctx context.Context, ln *net.TCPListener) (net.Listener, error) {
			return ln, nil
		})

		var requestCount atomic.Int32
		handlerBuilder := bedrock.BuilderFunc[http.Handler](func(ctx context.Context) (http.Handler, error) {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount.Add(1)
				w.WriteHeader(http.StatusOK)
			}), nil
		})

		runtimeBuilder := Build(listenerBuilder, handlerBuilder,
			DisableGeneralOptionsHandler(config.ReaderOf(false)),
			ReadTimeout(config.ReaderOf(5*time.Second)),
			ReadHeaderTimeout(config.ReaderOf(2*time.Second)),
			WriteTimeout(config.ReaderOf(10*time.Second)),
			IdleTimeout(config.ReaderOf(120*time.Second)),
			MaxHeaderBytes(config.ReaderOf(1048576)),
		)
		ctx := context.Background()
		runtime, err := runtimeBuilder.Build(ctx)
		require.NoError(t, err)

		addr := runtime.ls.Addr().String()

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- runtime.Run(runCtx)
		}()

		time.Sleep(100 * time.Millisecond)

		// Make multiple concurrent requests
		const numRequests = 10
		done := make(chan bool, numRequests)
		for i := 0; i < numRequests; i++ {
			go func() {
				resp, err := http.Get("http://" + addr)
				if err == nil {
					resp.Body.Close()
					done <- resp.StatusCode == http.StatusOK
				} else {
					done <- false
				}
			}()
		}

		// Wait for all requests to complete
		successCount := 0
		for i := 0; i < numRequests; i++ {
			if <-done {
				successCount++
			}
		}
		require.Equal(t, numRequests, successCount)

		cancel()

		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for shutdown")
		}
	})

	t.Run("graceful shutdown on context cancel", func(t *testing.T) {
		listenerBuilder := bedrock.Map(BuildTCPListener(config.ReaderOf(&net.TCPAddr{Port: 0})), func(ctx context.Context, ln *net.TCPListener) (net.Listener, error) {
			return ln, nil
		})

		shutdownCalled := make(chan struct{})
		handlerBuilder := bedrock.BuilderFunc[http.Handler](func(ctx context.Context) (http.Handler, error) {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				<-shutdownCalled
				w.WriteHeader(http.StatusOK)
			}), nil
		})

		runtimeBuilder := Build(listenerBuilder, handlerBuilder,
			DisableGeneralOptionsHandler(config.ReaderOf(false)),
			ReadTimeout(config.ReaderOf(5*time.Second)),
			ReadHeaderTimeout(config.ReaderOf(2*time.Second)),
			WriteTimeout(config.ReaderOf(10*time.Second)),
			IdleTimeout(config.ReaderOf(120*time.Second)),
			MaxHeaderBytes(config.ReaderOf(1048576)),
		)
		ctx := context.Background()
		runtime, err := runtimeBuilder.Build(ctx)
		require.NoError(t, err)

		addr := runtime.ls.Addr().String()

		runCtx, cancel := context.WithCancel(context.Background())

		errCh := make(chan error, 1)
		go func() {
			errCh <- runtime.Run(runCtx)
		}()

		time.Sleep(100 * time.Millisecond)

		// Start blocking request
		requestDone := make(chan struct{})
		go func() {
			resp, err := http.Get("http://" + addr)
			if err == nil {
				resp.Body.Close()
			}
			close(requestDone)
		}()

		time.Sleep(50 * time.Millisecond)

		// Cancel context (initiate shutdown)
		cancel()

		// Allow request to complete
		close(shutdownCalled)

		// Wait for request completion
		select {
		case <-requestDone:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for request")
		}

		// Wait for server shutdown
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for shutdown")
		}
	})

	t.Run("suppresses context.Canceled error", func(t *testing.T) {
		listenerBuilder := bedrock.Map(BuildTCPListener(config.ReaderOf(&net.TCPAddr{Port: 0})), func(ctx context.Context, ln *net.TCPListener) (net.Listener, error) {
			return ln, nil
		})

		handlerBuilder := bedrock.BuilderFunc[http.Handler](func(ctx context.Context) (http.Handler, error) {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil
		})

		runtimeBuilder := Build(listenerBuilder, handlerBuilder,
			DisableGeneralOptionsHandler(config.ReaderOf(false)),
			ReadTimeout(config.ReaderOf(5*time.Second)),
			ReadHeaderTimeout(config.ReaderOf(2*time.Second)),
			WriteTimeout(config.ReaderOf(10*time.Second)),
			IdleTimeout(config.ReaderOf(120*time.Second)),
			MaxHeaderBytes(config.ReaderOf(1048576)),
		)
		ctx := context.Background()
		runtime, err := runtimeBuilder.Build(ctx)
		require.NoError(t, err)

		// Create pre-canceled context
		runCtx, cancel := context.WithCancel(context.Background())
		cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- runtime.Run(runCtx)
		}()

		select {
		case err := <-errCh:
			require.NoError(t, err) // Should suppress context.Canceled
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for Run to return")
		}
	})
}
