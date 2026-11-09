// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/pki"
	"github.com/z5labs/bedrock/pki/pkitest"
)

// files is an in-memory directory, safe for concurrent use, that the tests
// rotate certificates and keys in. The module requires no other bedrock
// module, so it stands in for hosttest's.
type files struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newFiles(m map[string][]byte) *files {
	return &files{files: m}
}

func (f *files) ReadFile(name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.files[name]
	if !ok {
		return nil, errors.New("no such file: " + name)
	}
	return data, nil
}

func (f *files) WriteFile(name string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[name] = data
}

// pairLoader returns a load function that reads a key pair from the two
// files, as a config chain would.
func pairLoader(fsys *files, certFile, keyFile string) func(context.Context) (tls.Certificate, error) {
	return func(context.Context) (tls.Certificate, error) {
		certPEM, err := fsys.ReadFile(certFile)
		if err != nil {
			return tls.Certificate{}, err
		}
		keyPEM, err := fsys.ReadFile(keyFile)
		if err != nil {
			return tls.Certificate{}, err
		}
		return pki.KeyPair(certPEM, keyPEM)
	}
}

// served runs a TLS handshake over net.Pipe between a server with server
// and a client that trusts roots, and returns the leaf the server
// presented. It is safe to call from any goroutine of the test, because it
// reports a failure as an error rather than through t.
func served(ctx context.Context, server *tls.Config, roots *x509.CertPool) (*x509.Certificate, error) {
	srvConn, cliConn := net.Pipe()
	srvErr := make(chan error, 1)
	go func() {
		err := tls.Server(srvConn, server).HandshakeContext(ctx)
		srvErr <- errors.Join(err, srvConn.Close())
	}()
	cli := tls.Client(cliConn, &tls.Config{ServerName: "api.internal", RootCAs: roots})
	err := cli.HandshakeContext(ctx)
	err = errors.Join(err, cliConn.Close(), <-srvErr)
	if err != nil {
		return nil, err
	}
	return cli.ConnectionState().PeerCertificates[0], nil
}

func TestNewReloader_holds_the_value_it_is_given(t *testing.T) {
	r := pki.NewReloader(1, func(context.Context) (int, error) {
		require.FailNow(t, "NewReloader called load")
		return 0, nil
	})
	require.Equal(t, 1, r.Value())
}

func TestReloader_Reload_replaces_the_value_when_load_succeeds(t *testing.T) {
	next := 1
	r := pki.NewReloader(0, func(context.Context) (int, error) {
		v := next
		next++
		return v, nil
	})

	require.NoError(t, r.Reload(t.Context()))
	require.Equal(t, 1, r.Value())
	require.NoError(t, r.Reload(t.Context()))
	require.Equal(t, 2, r.Value())
}

// loadError is an error that a config chain might return. It is a
// pointer, so == holds only for the very error load returned, not for a
// wrapped or rebuilt one.
type loadError struct{ name string }

func (e *loadError) Error() string { return "config " + e.name + ": unreadable" }

func TestReloader_Reload_keeps_the_value_and_returns_loads_error_when_load_fails(t *testing.T) {
	errLoad := &loadError{name: "TLS_KEY_FILE"}
	var ctxs []context.Context
	r := pki.NewReloader("first", func(ctx context.Context) (string, error) {
		ctxs = append(ctxs, ctx)
		return "partial", errLoad
	})

	ctx := context.WithValue(t.Context(), ctxKey{}, "reload")
	err := r.Reload(ctx)
	require.Error(t, err)
	require.True(t, err == error(errLoad), "Reload returns the very error load returned")
	require.ErrorIs(t, err, errLoad)
	require.Equal(t, "first", r.Value(), "a failed load leaves the value unchanged")
	require.Equal(t, []context.Context{ctx}, ctxs, "load is called with Reload's context")
}

type ctxKey struct{}

func TestReloader_runs_one_Reload_at_a_time(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := []chan struct{}{make(chan struct{}), make(chan struct{})}
		var calls, inLoad atomic.Int32
		var overlapped atomic.Bool
		r := pki.NewReloader(0, func(context.Context) (int, error) {
			n := calls.Add(1)
			if inLoad.Add(1) > 1 {
				overlapped.Store(true)
			}
			defer inLoad.Add(-1)
			<-release[n-1]
			return int(n), nil
		})

		var wg sync.WaitGroup
		var errs [2]error
		wg.Go(func() { errs[0] = r.Reload(t.Context()) })
		synctest.Wait()
		require.Equal(t, int32(1), calls.Load(), "the first Reload is blocked in load")

		wg.Go(func() { errs[1] = r.Reload(t.Context()) })
		synctest.Wait()
		require.Equal(t, int32(1), calls.Load(), "the second Reload waits for the first")
		require.Equal(t, 0, r.Value(), "Value returns the old value without waiting")

		close(release[0])
		synctest.Wait()
		require.Equal(t, int32(2), calls.Load(), "the second Reload calls load once the first is done")
		require.Equal(t, 1, r.Value(), "the first's value is held while the second is in load")

		close(release[1])
		wg.Wait()
		require.NoError(t, errs[0])
		require.NoError(t, errs[1])
		require.Equal(t, 2, r.Value(), "the second's value is held")
		require.False(t, overlapped.Load(), "no two loads ran at once")
	})
}

func TestGetCertificate_serves_the_old_or_the_new_pair_while_Reload_replaces_it(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ca := pkitest.NewCA(t)
		old := ca.Issue(t, "api.internal").TLS()
		next := ca.Issue(t, "api.internal").TLS()
		pairs := make(chan tls.Certificate, 1)
		pairs <- next
		r := pki.NewReloader(old, func(context.Context) (tls.Certificate, error) {
			return <-pairs, nil
		})
		cfg := &tls.Config{GetCertificate: pki.GetCertificate(r)}

		const handshakes = 16
		leaves := make([]*x509.Certificate, handshakes)
		errs := make([]error, handshakes)
		var reloadErr error
		var wg sync.WaitGroup
		for i := range handshakes {
			wg.Go(func() { leaves[i], errs[i] = served(t.Context(), cfg, ca.Pool()) })
			if i == handshakes/2 {
				wg.Go(func() { reloadErr = r.Reload(t.Context()) })
			}
		}
		wg.Wait()

		require.NoError(t, reloadErr)
		for i, leaf := range leaves {
			require.NoError(t, errs[i])
			require.True(t, leaf.Equal(old.Leaf) || leaf.Equal(next.Leaf), "handshake %d saw the old leaf or the new one", i)
		}
		leaf, err := served(t.Context(), cfg, ca.Pool())
		require.NoError(t, err)
		require.True(t, leaf.Equal(next.Leaf), "a handshake after the Reload sees the new leaf")
	})
}

func TestGetClientCertificate_presents_the_pair_the_Reloader_holds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ca := pkitest.NewCA(t)
		first := ca.Issue(t).TLS()
		second := ca.Issue(t).TLS()
		r := pki.NewReloader(first, func(context.Context) (tls.Certificate, error) {
			return second, nil
		})
		server := &tls.Config{
			Certificates: []tls.Certificate{ca.Issue(t, "api.internal").TLS()},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    ca.Pool(),
		}
		client := &tls.Config{
			ServerName:           "api.internal",
			RootCAs:              ca.Pool(),
			GetClientCertificate: pki.GetClientCertificate(r),
		}

		srvState, _, srvErr, cliErr := handshake(t, server, client)
		require.NoError(t, cliErr)
		require.NoError(t, srvErr)
		require.True(t, srvState.PeerCertificates[0].Equal(first.Leaf), "the client presented the pair it holds")

		require.NoError(t, r.Reload(t.Context()))
		srvState, _, srvErr, cliErr = handshake(t, server, client)
		require.NoError(t, cliErr)
		require.NoError(t, srvErr)
		require.True(t, srvState.PeerCertificates[0].Equal(second.Leaf), "the client presented the reloaded pair")
	})
}

func TestReloader_holds_a_new_pool_for_GetConfigForClient(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		serverCA := pkitest.NewCA(t)
		oldCA := pkitest.NewCA(t)
		newCA := pkitest.NewCA(t)
		fsys := newFiles(map[string][]byte{"tls/client-ca.crt": oldCA.PEM()})
		load := func(context.Context) (*x509.CertPool, error) {
			data, err := fsys.ReadFile("tls/client-ca.crt")
			if err != nil {
				return nil, err
			}
			return pki.Pool(data)
		}
		first, err := load(t.Context())
		require.NoError(t, err)
		pools := pki.NewReloader(first, load)

		base := &tls.Config{
			Certificates: []tls.Certificate{serverCA.Issue(t, "api.internal").TLS()},
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}
		server := &tls.Config{
			GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
				cfg := base.Clone()
				cfg.ClientCAs = pools.Value()
				return cfg, nil
			},
		}
		// accepts reports whether the server accepts a client whose
		// certificate ca issued.
		accepts := func(ca *pkitest.CA) bool {
			_, _, srvErr, _ := handshake(t, server, &tls.Config{
				ServerName:   "api.internal",
				RootCAs:      serverCA.Pool(),
				Certificates: []tls.Certificate{ca.Issue(t).TLS()},
			})
			return srvErr == nil
		}
		require.True(t, accepts(oldCA), "before the reload, a client of the old CA is accepted")
		require.False(t, accepts(newCA), "before the reload, a client of the new CA is rejected")

		fsys.WriteFile("tls/client-ca.crt", newCA.PEM())
		require.NoError(t, pools.Reload(t.Context()))
		require.True(t, newCA.Pool().Equal(pools.Value()), "the Reloader holds the new pool")
		require.True(t, accepts(newCA), "after the reload, a client of the new CA is accepted")
		require.False(t, accepts(oldCA), "after the reload, a client of the old CA is rejected")
	})
}

func TestRotatingTLS_serves_the_pair_it_last_read(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ca := pkitest.NewCA(t)
		certPEM, keyPEM := ca.Issue(t, "api.internal").PEM()
		fsys := newFiles(map[string][]byte{
			"tls/tls.crt": certPEM,
			"tls/tls.key": keyPEM,
		})
		pair := pairLoader(fsys, "tls/tls.crt", "tls/tls.key")
		first, err := pair(t.Context())
		require.NoError(t, err)
		certs := pki.NewReloader(first, pair)
		cfg := &tls.Config{GetCertificate: pki.GetCertificate(certs)}

		// serve returns the leaf that the server presented in a new
		// handshake.
		serve := func() *x509.Certificate {
			leaf, err := served(t.Context(), cfg, ca.Pool())
			require.NoError(t, err)
			return leaf
		}
		require.True(t, serve().Equal(first.Leaf))

		next := ca.Issue(t, "api.internal")
		certPEM, keyPEM = next.PEM()
		fsys.WriteFile("tls/tls.crt", certPEM)
		fsys.WriteFile("tls/tls.key", keyPEM)
		require.NoError(t, certs.Reload(t.Context()))
		require.False(t, serve().Equal(first.Leaf))
		require.True(t, serve().Equal(next.TLS().Leaf))

		torn, _ := ca.Issue(t, "api.internal").PEM()
		fsys.WriteFile("tls/tls.crt", torn)
		require.ErrorContains(t, certs.Reload(t.Context()), "private key does not match public key")
		require.True(t, serve().Equal(next.TLS().Leaf), "a torn rotation keeps the last good pair")
	})
}

func TestNewReloader_panics_if_load_is_nil(t *testing.T) {
	require.PanicsWithValue(t, "pki: NewReloader: load is nil", func() {
		pki.NewReloader[int](0, nil)
	})
}

func TestGetCertificate_panics_if_the_Reloader_is_nil(t *testing.T) {
	require.PanicsWithValue(t, "pki: GetCertificate: r is nil", func() {
		pki.GetCertificate(nil)
	})
}

func TestGetClientCertificate_panics_if_the_Reloader_is_nil(t *testing.T) {
	require.PanicsWithValue(t, "pki: GetClientCertificate: r is nil", func() {
		pki.GetClientCertificate(nil)
	})
}
