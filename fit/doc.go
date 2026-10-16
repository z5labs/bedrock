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
// # No wire convention
//
// fit owns no wire convention. The scenario travels as baggage, and the
// codec the caller supplies decides the baggage keys, the grammar of the
// encoded scenario, how large it may be, when it expires and whether it is
// trusted. The edge that decides which requests are under test, such as
// Envoy, Zuul or Kong, is usually owned by a different team, so fit forces
// none of those choices on it. Likewise fit puts no limit on the
// characters of an injection point's name.
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
