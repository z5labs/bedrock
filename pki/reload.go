// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki

import (
	"context"
	"crypto/tls"
	"sync/atomic"
)

// Reloader holds a value, such as a key pair from KeyPair or a pool from
// Pool, and replaces it each time Reload reads a new one. Its methods are
// safe for concurrent use.
//
// A Reloader starts no goroutine, reads no clock and watches no file. When
// to call Reload is the caller's choice: on a ticker, after a file watcher
// fires, or when an operator asks.
//
// Make a Reloader with NewReloader. The zero Reloader is not ready for use:
// its Value panics and its Reload never returns.
type Reloader[T any] struct {
	load func(context.Context) (T, error)

	// lock is a mutex, held across the whole of a Reload by a send and
	// released by a receive, so reloads run one at a time. Value never
	// takes it. It is a one-slot channel rather than a sync.Mutex because a
	// goroutine waiting on a channel is durably blocked in a
	// testing/synctest bubble and one waiting on a sync.Mutex isn't, so
	// with a sync.Mutex, synctest.Wait in a test that overlaps two reloads
	// would never return.
	lock chan struct{}

	// v points to the value held. A value is never modified once it is
	// stored, so the pointer can be handed to crypto/tls as it is.
	v atomic.Pointer[T]
}

// NewReloader returns a Reloader that holds v and reads new values with
// load. The caller reads v, typically with the config chain that load
// reads again, so a value that can't be read fails at startup with the
// rest of the configuration. NewReloader panics if load is nil.
func NewReloader[T any](v T, load func(context.Context) (T, error)) *Reloader[T] {
	if load == nil {
		panic("pki: NewReloader: load is nil")
	}
	r := &Reloader[T]{load: load, lock: make(chan struct{}, 1)}
	r.v.Store(&v)
	return r
}

// Reload calls load. If load succeeds, the value it returns replaces the
// one that the Reloader holds. If load fails, the Reloader keeps its
// value, and Reload returns load's error unchanged. Reloads run one at a
// time, so a slow load never replaces a newer value.
func (r *Reloader[T]) Reload(ctx context.Context) error {
	r.lock <- struct{}{}
	defer func() { <-r.lock }()

	v, err := r.load(ctx)
	if err != nil {
		// load's error is returned as it is, so that a config chain's
		// name for it, and errors.Is and ==, all still see it.
		return err
	}
	r.v.Store(&v)
	return nil
}

// Value returns the value that the Reloader holds. It never waits for a
// Reload.
func (r *Reloader[T]) Value() T {
	return *r.v.Load()
}

// GetCertificate returns a function for a server's
// tls.Config.GetCertificate, which serves the key pair that r holds at
// each handshake. Leave the config's Certificates empty, because a
// handshake from a client that sends no server name uses Certificates
// instead.
//
// A Reload reaches new handshakes only. GetCertificate panics if r is nil.
func GetCertificate(r *Reloader[tls.Certificate]) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	if r == nil {
		panic("pki: GetCertificate: r is nil")
	}
	return func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		return r.v.Load(), nil
	}
}

// GetClientCertificate returns a function for a client's
// tls.Config.GetClientCertificate, which presents the key pair that r
// holds whenever a server asks for one.
//
// A Reload reaches new handshakes only. GetClientCertificate panics if r
// is nil.
func GetClientCertificate(r *Reloader[tls.Certificate]) func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	if r == nil {
		panic("pki: GetClientCertificate: r is nil")
	}
	return func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		return r.v.Load(), nil
	}
}
