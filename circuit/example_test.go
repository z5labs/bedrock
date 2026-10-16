// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/z5labs/bedrock/circuit"
)

// classify counts every error against the dependency, every deadline
// included, except one the caller caused by giving up.
func classify(ctx context.Context, err error) circuit.Outcome {
	switch {
	case err == nil:
		return circuit.Success
	case errors.Is(ctx.Err(), context.Canceled):
		return circuit.Ignored
	default:
		return circuit.Failure
	}
}

// consecutive opens after n failures in a row, reopens on any failed probe,
// and closes after n successful probes in a row.
func consecutive(n int) circuit.Policy {
	return circuit.Policy{
		Trip: func(c circuit.Counts) bool { return c.ConsecutiveFailures >= n },
		Probe: func(c circuit.Counts) circuit.State {
			switch {
			case c.Failures > 0:
				return circuit.Open
			case c.ConsecutiveSuccesses >= n:
				return circuit.Closed
			default:
				return circuit.HalfOpen
			}
		},
	}
}

func printTransition(_ context.Context, t circuit.Transition) {
	fmt.Printf("observe: #%d %v -> %v after %d failed, %d succeeded\n",
		t.Generation, t.From, t.To, t.Counts.Failures, t.Counts.Successes)
}

// cfg would come from configuration. Its open period is short so that the
// examples run quickly.
var cfg = circuit.Config{Window: 20, Probes: 1, OpenFor: 10 * time.Millisecond, ProbeTimeout: time.Second}

// payments is a client of a payments service that calls it through a
// Breaker. The Breaker sits inline in Charge, around the request.
type payments struct {
	breaker *circuit.Breaker
	results []error // how the service answers, one call at a time
}

// Charge is an outbound call. Allow admits it, the deferred Release guards
// it, and Done settles it with the result before anything checks it.
func (p *payments) Charge(ctx context.Context) error {
	t, err := p.breaker.Allow(ctx)
	if err != nil {
		return err // a *circuit.RejectedError
	}
	defer t.Release()

	err = p.send(ctx)
	t.Done(err)
	return err
}

// send stands in for the request to the service.
func (p *payments) send(context.Context) error {
	err := p.results[0]
	p.results = p.results[1:]
	return err
}

// Two failures in a row open the Breaker, and it rejects calls without
// making them until its open period has passed. Then it is half-open, and
// two successful probes close it.
func Example() {
	ctx := context.Background()

	// At startup.
	b, err := circuit.New(cfg, consecutive(2), classify, printTransition)
	if err != nil {
		fmt.Println(err)
		return
	}
	unavailable := errors.New("payments: unavailable")
	p := &payments{breaker: b, results: []error{unavailable, unavailable, nil, nil}}

	// Per outbound call.
	report := func(err error) {
		if rej, ok := errors.AsType[*circuit.RejectedError](err); ok {
			fmt.Println("charge: rejected while", rej.State)
			return
		}
		fmt.Println("charge:", err)
	}
	for range 3 {
		report(p.Charge(ctx))
	}

	time.Sleep(cfg.OpenFor)
	fmt.Println("state:", b.State())
	for range 2 {
		report(p.Charge(ctx))
	}
	fmt.Println("state:", b.State())
	// Output:
	// charge: payments: unavailable
	// observe: #1 closed -> open after 2 failed, 0 succeeded
	// charge: payments: unavailable
	// charge: rejected while open
	// state: half-open
	// observe: #2 open -> half-open after 0 failed, 0 succeeded
	// charge: <nil>
	// observe: #3 half-open -> closed after 0 failed, 2 succeeded
	// charge: <nil>
	// state: closed
}

// body settles its Ticket once, whichever comes first: the end of the body,
// an error while reading it, or Close. Done panics on a Ticket that is
// already settled, so the sync.Once is required.
type body struct {
	io.ReadCloser
	once   sync.Once
	ticket *circuit.Ticket
}

func (b *body) settle(err error) { b.once.Do(func() { b.ticket.Done(err) }) }

func (b *body) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	switch {
	case errors.Is(err, io.EOF):
		b.settle(nil)
	case err != nil:
		b.settle(err) // the body failed partway through
	}
	return n, err
}

func (b *body) Close() error {
	b.settle(nil) // the caller stopped reading, having got what it needed
	return b.ReadCloser.Close()
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// transport sends requests to next through a Breaker. The result of a
// request is known only once the caller has read the body, after RoundTrip
// returns, so the body settles the Ticket, and RoundTrip guards it with a
// hand-off flag instead of a plain deferred Release.
type transport struct {
	breaker *circuit.Breaker
	next    http.RoundTripper
}

func (tr transport) RoundTrip(req *http.Request) (*http.Response, error) {
	t, err := tr.breaker.Allow(req.Context())
	if err != nil {
		return nil, err
	}
	handedOff := false
	defer func() {
		if !handedOff {
			t.Release() // next panicked, or called runtime.Goexit
		}
	}()

	resp, err := tr.next.RoundTrip(req)
	if err != nil {
		t.Done(err)
		return nil, err
	}
	resp.Body = &body{ReadCloser: resp.Body, ticket: t}
	handedOff = true
	return resp, nil
}

// A response whose body fails while the caller reads it counts as a
// failure. Allow admits the request when it starts, and the body settles
// the Ticket when the result is known.
func ExampleBreaker_Allow_streamedBody() {
	b, err := circuit.New(cfg, consecutive(1), classify, printTransition)
	if err != nil {
		fmt.Println(err)
		return
	}
	client := &http.Client{Transport: transport{
		breaker: b,
		next: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(io.MultiReader(
				strings.NewReader("first chunk, "),
				errReader{errors.New("connection reset")},
			))}, nil
		}),
	}}

	resp, err := client.Get("http://payments.invalid/")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("status:", resp.StatusCode, b.State())
	got, err := io.ReadAll(resp.Body)
	fmt.Printf("read %q: %v\n", got, err)
	fmt.Println("close:", resp.Body.Close())
	fmt.Println("state:", b.State())
	// Output:
	// status: 200 closed
	// observe: #1 closed -> open after 1 failed, 0 succeeded
	// read "first chunk, ": connection reset
	// close: <nil>
	// state: open
}

// A result that arrives after the Breaker has changed state belongs to a
// state that has ended, so it is discarded rather than counted in the newer
// one, and Done reports false.
func ExampleTicket_Done() {
	b, err := circuit.New(cfg, consecutive(1), classify, printTransition)
	if err != nil {
		fmt.Println(err)
		return
	}
	ctx := context.Background()

	slow, err := b.Allow(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer slow.Release()
	fast, err := b.Allow(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer fast.Release()

	fmt.Println("fast counted:", fast.Done(errors.New("connection refused")))
	fmt.Println("slow counted:", slow.Done(nil))
	fmt.Println("state:", b.State())
	// Output:
	// observe: #1 closed -> open after 1 failed, 0 succeeded
	// fast counted: true
	// slow counted: false
	// state: open
}

// http.Client.Timeout is a deadline on the context of the request, which is
// the context the Breaker sees, so a dependency that hangs opens the
// Breaker under this classifier. A request whose caller cancels it is
// Ignored.
func ExampleNew_classify() {
	b, err := circuit.New(cfg, consecutive(2), classify, printTransition)
	if err != nil {
		fmt.Println(err)
		return
	}
	hang := func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	}
	client := &http.Client{
		Timeout: 100 * time.Millisecond,
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			t, err := b.Allow(r.Context())
			if err != nil {
				return nil, err
			}
			defer t.Release()

			resp, err := hang(r) // hang returns no body, so its result is known now
			t.Done(err)
			return resp, err
		}),
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(cancelled, http.MethodGet, "http://payments.invalid/", nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	_, err = client.Do(req)
	fmt.Println("cancelled:", errors.Is(err, context.Canceled), b.State())

	for range 3 {
		_, err := client.Get("http://payments.invalid/")
		fmt.Println("timed out:", errors.Is(err, context.DeadlineExceeded), "rejected:", errors.Is(err, circuit.ErrRejected))
	}
	// Output:
	// cancelled: true closed
	// timed out: true rejected: false
	// observe: #1 closed -> open after 2 failed, 0 succeeded
	// timed out: true rejected: false
	// timed out: false rejected: true
}

// A Policy decides from Counts alone, so it can be tested without a
// Breaker. This one opens on a run of failures or on a failure ratio over a
// minimum number of results, and keeps probing when a probe times out.
func ExamplePolicy() {
	p := circuit.Policy{
		Trip: func(c circuit.Counts) bool {
			n := c.Successes + c.Failures
			return c.ConsecutiveFailures >= 5 || (n >= 20 && 2*c.Failures >= n)
		},
		Probe: func(c circuit.Counts) circuit.State {
			switch {
			case c.Failures > 0:
				return circuit.Open
			case c.ConsecutiveSuccesses >= 3:
				return circuit.Closed
			default:
				return circuit.HalfOpen
			}
		},
	}

	fmt.Println(p.Trip(circuit.Counts{Successes: 9, Failures: 9, ConsecutiveFailures: 1}))
	fmt.Println(p.Trip(circuit.Counts{Successes: 10, Failures: 10, ConsecutiveFailures: 1}))
	fmt.Println(p.Trip(circuit.Counts{Successes: 40, Failures: 5, ConsecutiveFailures: 5}))
	fmt.Println(p.Probe(circuit.Counts{TimedOut: 1}))
	fmt.Println(p.Probe(circuit.Counts{Successes: 3, ConsecutiveSuccesses: 3, TimedOut: 1}))
	fmt.Println(p.Probe(circuit.Counts{Successes: 2, Failures: 1, ConsecutiveFailures: 1}))
	// Output:
	// false
	// true
	// true
	// half-open
	// closed
	// open
}

// Every field of Config is required, and New reports every invalid one at
// once, so App.Run can print the error and exit non-zero at startup.
func ExampleNew() {
	_, err := circuit.New(circuit.Config{Probes: 1, OpenFor: -time.Second}, consecutive(5), classify, printTransition)
	fmt.Println(err)
	// Output:
	// circuit: window must be between 1 and 65536, got 0
	// circuit: open duration must be positive, got -1s
	// circuit: probe timeout must be positive, got 0s
}

// observe runs with the Breaker unlocked, one Transition at a time and in
// order, so it may call the Breaker, and a gauge it sets needs no lock.
func ExampleNew_observe() {
	var b *circuit.Breaker
	observe := func(_ context.Context, t circuit.Transition) {
		fmt.Printf("gauge: %v (State says %v)\n", t.To, b.State())
	}
	b, err := circuit.New(cfg, consecutive(1), classify, observe)
	if err != nil {
		fmt.Println(err)
		return
	}
	ctx := context.Background()
	p := &payments{breaker: b, results: []error{errors.New("unavailable"), nil}}

	_ = p.Charge(ctx)
	time.Sleep(cfg.OpenFor)
	_ = p.Charge(ctx)
	// Output:
	// gauge: open (State says open)
	// gauge: half-open (State says half-open)
	// gauge: closed (State says closed)
}

// Code between Allow and Done may panic, and net/http's server recovers a
// panic in a handler. The Release deferred straight after Allow frees the
// probe's slot on the way out, and calls nothing that could panic again,
// so the Breaker goes on probing.
func ExampleTicket_Release() {
	b, err := circuit.New(cfg, consecutive(1), classify, func(context.Context, circuit.Transition) {})
	if err != nil {
		fmt.Println(err)
		return
	}
	p := &payments{breaker: b, results: []error{errors.New("unavailable"), nil}}
	_ = p.Charge(context.Background())
	time.Sleep(cfg.OpenFor)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t, err := b.Allow(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		defer t.Release()

		id := strings.Split(r.URL.Path, "/")[2] // a bug: "/" has no second segment, so this panics
		err = p.send(r.Context())
		t.Done(err)
		w.Header().Set("X-Id", id)
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.Start()
	defer srv.Close()

	_, err = http.Get(srv.URL)
	fmt.Println("request failed:", err != nil)
	_, err = b.Allow(context.Background())
	fmt.Println("next probe admitted:", err == nil)
	// Output:
	// request failed: true
	// next probe admitted: true
}
