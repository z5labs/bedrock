// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package fit models request-scoped failure injection.
//
// # Background
//
// fit follows Netflix's Failure Injection Testing, described in
// https://netflixtechblog.com/fit-failure-injection-testing-35d8e2a9bb2
// (2014). A request that is under test carries a failure scenario with it,
// and every injection point the request passes through, such as an
// outgoing call or a database query, looks itself up in that scenario to
// learn whether it should fail. Only requests that carry a scenario are
// affected, so a failure can be injected into production traffic one
// request at a time.
//
// A [Scenario] is an ordered list of [Rule] values, and the first Rule
// whose Point matches an injection point decides the [Fault] that point
// emulates. Because the zero Fault spares a point, one list covers both of
// FIT's scenario shapes:
//
//	// Fail the payments call.
//	fit.Scenario{Rules: []fit.Rule{
//		{Point: "payments", Fault: fit.Fault{Abort: true}},
//	}}
//
//	// Fail everything except the payments call.
//	fit.Scenario{Rules: []fit.Rule{
//		{Point: "payments"},
//		{Point: "*", Fault: fit.Fault{Abort: true}},
//	}}
//
// An injection point emulates its Fault with [Fault.Inject], or with
// [Fault.Wait] and its own failure when it can fail more realistically.
//
// # Injection points
//
// An injection point asks an [Evaluator] which Fault to emulate for the
// current request:
//
//	if err := eval.Eval(ctx, "http:payments").Inject(ctx); err != nil {
//		return err
//	}
//
// [Evaluator.Eval] never fails a request because of its test metadata. A
// request that carries no Scenario gets the zero Fault. A Scenario the
// [Decoder] rejects, for example because it has expired or names an unknown
// scenario, also gives the zero Fault, and the Decoder's error goes to the
// observe function passed to [NewEvaluator] in [Evaluation.Err]. A Delay
// longer than the Evaluator's maximum is clamped to it rather than rejected,
// so services with different maximums still agree on the rest of a Scenario.
// A Scenario with no rules injects nothing, yet observe sees every point the
// request reaches, which is how the points a request passes through are
// discovered.
//
// fit ships no observe function. Recording an [Evaluation] as a span event,
// a log line or a metric, and naming its attributes, is a convention of the
// organisation that builds on fit.
//
// Each call belongs either at startup or on the request path:
//
//	| Call                                                                  | When                                                   |
//	|-----------------------------------------------------------------------|--------------------------------------------------------|
//	| Building the Decoder/Encoder (keys, registry, clock) and NewEvaluator | Startup, inside App.Run                                |
//	| Point names fixed per client, such as a client bound to one host      | When the client is built                               |
//	| ContextWithoutScenario on requests from untrusted callers             | Per request, at ingress                                |
//	| ContextWithScenario                                                   | Per request, at the edge                               |
//	| Eval, then Inject, or Wait plus a protocol-specific abort             | Per outbound call                                      |
//	| observe                                                               | Per evaluation, only on requests that carry a scenario |
//	| ContextWithoutScenario before calling a third party                   | Per outbound call                                      |
//
// One Evaluator is shared by every request goroutine, so its Decoder and its
// observe function must be safe for concurrent use.
//
// Decode runs on every call to Eval, so a request under test that reaches N
// injection points is decoded N times. A Decoder whose Decode is costly, such
// as one that verifies a signature, should memoize internally, keyed by the
// raw member value. A Decoder that checks expiry decides whether a Scenario
// that expires partway through a request still applies.
//
// # No wire convention
//
// fit owns no wire convention. The scenario travels as OpenTelemetry
// baggage, and the codec the caller supplies decides the baggage keys, the
// grammar of the encoded scenario, its version, how large it may be, when
// it expires, which injection point names it accepts and whether it is
// trusted. The edge that decides which requests are under test, such as
// Envoy, Zuul or Kong, is usually owned by a different team, so fit forces
// none of those choices on it. fit declares no baggage key, value grammar,
// size limit, signature scheme or expiry of its own, and likewise puts no
// limit on the characters of an injection point's name.
//
// # Codecs
//
// A codec is split in two, because most services only read and strip
// scenarios and only the edge writes them:
//
//   - a [Decoder] reads a Scenario from baggage and strips it. Decode
//     reports ok == false when the baggage carries no Scenario, and a
//     non-nil error when it carries one that cannot be honoured. Strip
//     removes exactly the members Decode reads and leaves every other
//     member alone;
//   - an [Encoder] writes a Scenario into baggage, replacing any Scenario
//     the baggage already carries.
//
// Implementations of both must be safe for concurrent use.
//
// [ContextWithScenario], [ScenarioFromContext] and [ContextWithoutScenario]
// apply a codec to the baggage of a context. Moving that baggage between
// services stays the caller's job, typically with an OpenTelemetry
// propagator.
//
// # Trust
//
// OpenTelemetry baggage has no integrity check. Services commonly accept
// baggage from any caller, and OpenTelemetry auto-instrumentation forwards
// baggage to third parties. A scenario is an instruction to fail, so a
// deployment must decide whom it takes that instruction from:
//
//   - a deployment establishes trust in its codec, for example with a
//     signature or a registry of known scenario IDs, or by stripping
//     scenarios at its edge;
//   - [ContextWithoutScenario] belongs at both trust boundaries: on
//     requests from untrusted callers, and on calls out to third parties.
//
// Envoy's fault filter gives the same warning about its x-envoy-fault-*
// headers.
//
// # Out of scope
//
// fit does not:
//
//   - decide which requests are under test, whether by account, by device
//     or by percentage of traffic;
//   - move the failure context between services;
//   - provide a control plane for defining or distributing scenarios;
//   - fail a point with some probability on each call. A Fault either
//     applies to a point or it does not.
package fit
