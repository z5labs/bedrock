// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package health

import (
	"context"
	"net/http"
	"sync"
)

// Metric represents anything that can report its health status.
type Metric interface {
	Healthy(context.Context) bool
}

// Started is used for signifying that the application
// requires a longer amount of time to initialize.
type Started struct {
	mu      sync.RWMutex
	started bool
}

// Started marks this metric as "healthy".
func (s *Started) Started() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = true
}

// Healthy implements the Metric interface.
func (s *Started) Healthy(ctx context.Context) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.started
}

// ServeHTTP implements the http.Handler interface.
func (s *Started) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	started := s.Healthy(req.Context())
	if started {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
}

// Readiness is used for signifying that the application is
// temporarily unable to serve traffic.
type Readiness struct {
	mu    sync.RWMutex
	ready bool
}

// NotReady marks this metric as "unhealthy".
func (r *Readiness) NotReady() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ready = false
}

// Ready marks this metric as "healthy".
func (r *Readiness) Ready() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ready = true
}

// Healthy implements the Metric interface.
func (r *Readiness) Healthy(ctx context.Context) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready
}

// ServeHTTP implements the http.Handler interface.
func (r *Readiness) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ready := r.Healthy(req.Context())
	if ready {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
}

// Liveness is used for signifying that the application has transitioned
// to a broken state, and cannot recover execpt by being restarted.
type Liveness struct {
	mu    sync.RWMutex
	alive bool
}

// Dead marks this metric as "unhealthy".
func (l *Liveness) Dead() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.alive = false
}

// Alive marks this metric as "healthy".
func (l *Liveness) Alive() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.alive = true
}

// Healthy implements the Metric interface.
func (l *Liveness) Healthy(ctx context.Context) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.alive
}

// ServeHTTP implements the http.Handler interface.
func (l *Liveness) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	alive := l.Healthy(req.Context())
	if alive {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
}
