// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package bedrocktest provides a hermetic [bedrock.Process] for tests that
// run a whole [bedrock.App], in the way net/http/httptest provides servers
// for tests of HTTP handlers.
//
// A test builds a [Process] with [NewProcess], runs its App on
// [Process.Process], drives the App's signals with [Process.Notified] and
// [Process.Signal], and asserts on the returned status and on
// [Process.Stdout] and [Process.Stderr]:
//
//	fp := bedrocktest.NewProcess(t, bedrocktest.WithArgs("-addr", ":0"))
//	go func() { statusCh <- app.Run(t.Context(), fp.Process()) }()
//	<-fp.Notified(syscall.SIGTERM)
//	fp.Signal(syscall.SIGTERM)
//
// The fake touches nothing of the real process, so such a test can run in
// parallel, and it starts no goroutines, so it can run inside a
// testing/synctest bubble.
package bedrocktest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/z5labs/bedrock"
)

// ProgramName is the program name of every Process, which is Args[0] of
// the [bedrock.Process] it returns.
const ProgramName = "bedrocktest"

// Option configures a Process built by [NewProcess].
type Option func(*options)

type options struct {
	args  []string
	env   bedrock.Environment
	stdin io.Reader
}

// WithArgs sets the command-line arguments of the Process after the
// program name, so that Args is [ProgramName] followed by args. The
// default is no arguments. If WithArgs is given more than once, the last
// one wins.
func WithArgs(args ...string) Option {
	args = slices.Clone(args)
	return func(o *options) {
		o.args = args
	}
}

// WithEnv sets the environment of the Process to
// bedrock.NewStaticEnvironment(values), which copies values, so changing
// values afterwards does not change the environment. The default is an
// empty environment, in which every lookup returns a
// [bedrock.ValueNotFoundError]. If WithEnv is given more than once, the
// last one wins.
func WithEnv(values map[string]string) Option {
	env := bedrock.NewStaticEnvironment(values)
	return func(o *options) {
		o.env = env
	}
}

// WithStdin sets the standard input of the Process. The default is an
// empty reader, whose Read returns io.EOF. WithStdin panics if r is nil.
// If WithStdin is given more than once, the last one wins.
func WithStdin(r io.Reader) Option {
	if r == nil {
		panic("bedrocktest: WithStdin requires a non-nil io.Reader")
	}
	return func(o *options) {
		o.stdin = r
	}
}

// Process is a fake OS process for tests. It hands out a complete
// [bedrock.Process] through [Process.Process], records what the App writes
// to its standard streams, and delivers fake signals to the contexts the
// App derives through NotifyContext.
//
// Build one with [NewProcess]. A Process is safe for concurrent use.
type Process struct {
	tb     testing.TB
	args   []string
	env    bedrock.Environment
	stdin  io.Reader
	stdout *syncBuffer
	stderr *syncBuffer

	mu       sync.Mutex
	regs     []*registration
	notified map[os.Signal]chan struct{}
}

// NewProcess returns a Process configured by opts, reporting misuse
// through tb.
//
// By default Args is [ProgramName] alone, Stdin is empty and the
// environment has no variables set. Stdout and Stderr are in-memory and
// safe for concurrent writes.
//
// The fake is deliberately strict, so that a test fails where a real
// process would have misbehaved:
//
//   - [Process.Signal] fails the test through tb.Errorf when no live
//     NotifyContext is registered for the signal, because a real process
//     would have taken the signal's default action.
//   - A tb.Cleanup registered here fails the test when a context returned
//     by NotifyContext was never stopped.
//
// NewProcess starts no goroutines and allocates no channels: NotifyContext
// and [Process.Notified] create theirs when they are first called. To use a
// Process with testing/synctest, call NewProcess inside synctest.Test with
// the bubble's t, so that every channel the Process creates belongs to the
// bubble that uses it.
func NewProcess(tb testing.TB, opts ...Option) *Process {
	o := options{
		env:   bedrock.NewStaticEnvironment(nil),
		stdin: strings.NewReader(""),
	}
	for _, opt := range opts {
		opt(&o)
	}

	p := &Process{
		tb:     tb,
		args:   append([]string{ProgramName}, o.args...),
		env:    o.env,
		stdin:  o.stdin,
		stdout: new(syncBuffer),
		stderr: new(syncBuffer),
	}
	tb.Cleanup(p.checkStopped)
	return p
}

// Process returns the [bedrock.Process] to run an App on. Every field is
// set. Each call returns a fresh copy of Args, and the same streams,
// environment and signal delivery.
func (p *Process) Process() bedrock.Process {
	return bedrock.Process{
		Args:          slices.Clone(p.args),
		Stdin:         p.stdin,
		Stdout:        p.stdout,
		Stderr:        p.stderr,
		Environment:   p.env,
		NotifyContext: p.notifyContext,
	}
}

// Stdout returns a snapshot of everything written to the standard output
// of the Process so far. It is safe to call while the App is writing.
func (p *Process) Stdout() string {
	return p.stdout.String()
}

// Stderr returns a snapshot of everything written to the standard error of
// the Process so far. It is safe to call while the App is writing.
func (p *Process) Stderr() string {
	return p.stderr.String()
}

// Notified returns a channel that is closed once the App holds a live
// NotifyContext registration for sig, either one listing sig or one
// listing no signals. A test receives from it before calling
// [Process.Signal], so that the signal is not sent before the App is ready
// for it.
//
// Once closed, the channel stays closed. After every registration for sig
// has been stopped, Notified returns a new channel, closed when the next
// registration for sig is made.
func (p *Process) Notified(sig os.Signal) <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ch, ok := p.notified[sig]; ok {
		return ch
	}
	if p.notified == nil {
		p.notified = make(map[os.Signal]chan struct{})
	}
	ch := make(chan struct{})
	p.notified[sig] = ch
	if p.wantedLocked(sig) {
		close(ch)
	}
	return ch
}

// Signal delivers sig to the Process. Every live NotifyContext
// registration for sig is cancelled with a cause whose message is
// "<sig> signal received", such as "terminated signal received", and for
// which errors.Is(cause, context.Canceled) holds, as with
// signal.NotifyContext. A registration stays live until it is stopped, so
// signalling it again is not an error.
//
// If no live registration wants sig, Signal fails the test through
// tb.Errorf, because a real process would have taken the signal's default
// action, which for most signals is to exit.
func (p *Process) Signal(sig os.Signal) {
	p.tb.Helper()

	p.mu.Lock()
	var live []*registration
	for _, r := range p.regs {
		if r.wants(sig) {
			live = append(live, r)
		}
	}
	p.mu.Unlock()

	if len(live) == 0 {
		p.tb.Errorf("bedrocktest: Signal(%v) with no live NotifyContext registered for it; a real process would have taken the signal's default action", sig)
		return
	}

	cause := signalError(sig.String() + " signal received")
	for _, r := range live {
		r.cancel(cause)
	}
}

// notifyContext is the NotifyContext of the bedrock.Process. It follows
// signal.NotifyContext: the returned context derives from parent, listing
// no signals means every signal, and stop unregisters the context and
// cancels it with context.Canceled.
func (p *Process) notifyContext(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	r := &registration{
		signals: slices.Clone(signals),
		cancel:  cancel,
	}

	p.mu.Lock()
	p.regs = append(p.regs, r)
	for sig, ch := range p.notified {
		if r.wants(sig) && !isClosed(ch) {
			close(ch)
		}
	}
	p.mu.Unlock()

	return ctx, func() { p.stop(r) }
}

// stop unregisters r and cancels its context with context.Canceled. It is
// safe to call more than once.
func (p *Process) stop(r *registration) {
	p.mu.Lock()
	p.regs = slices.DeleteFunc(p.regs, func(live *registration) bool { return live == r })
	for sig, ch := range p.notified {
		if isClosed(ch) && !p.wantedLocked(sig) {
			delete(p.notified, sig)
		}
	}
	p.mu.Unlock()

	r.cancel(nil)
}

// checkStopped fails the test for every NotifyContext that was never
// stopped, then cancels them. It runs in tb.Cleanup.
func (p *Process) checkStopped() {
	p.tb.Helper()

	p.mu.Lock()
	live := p.regs
	p.regs = nil
	p.mu.Unlock()

	for _, r := range live {
		p.tb.Errorf("bedrocktest: a NotifyContext for %s was never stopped; call its stop function, typically with defer", r)
		r.cancel(nil)
	}
}

// wantedLocked reports whether any live registration wants sig. p.mu must
// be held.
func (p *Process) wantedLocked(sig os.Signal) bool {
	return slices.ContainsFunc(p.regs, func(r *registration) bool { return r.wants(sig) })
}

// registration is one live NotifyContext.
type registration struct {
	signals []os.Signal
	cancel  context.CancelCauseFunc
}

// wants reports whether a signal is delivered to r: it lists sig, or it
// lists no signals at all.
func (r *registration) wants(sig os.Signal) bool {
	return len(r.signals) == 0 || slices.Contains(r.signals, sig)
}

// String describes the signals r was registered for.
func (r *registration) String() string {
	if len(r.signals) == 0 {
		return "every signal"
	}
	return fmt.Sprint(r.signals)
}

// signalError is the cause of a context cancelled by a signal. Like the
// cause signal.NotifyContext uses, it matches context.Canceled under
// errors.Is.
type signalError string

func (e signalError) Error() string {
	return string(e)
}

func (signalError) Is(target error) bool {
	return target == context.Canceled
}

func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// syncBuffer is a bytes.Buffer that is safe for concurrent use, as an
// *os.File is.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
