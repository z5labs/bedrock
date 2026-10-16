// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package fit_test

import (
	"context"
	"fmt"

	"github.com/z5labs/bedrock/fit"
	"go.opentelemetry.io/otel/propagation"
)

// An Envoy edge cannot compute a signature, so it writes the scenario as a
// static baggage header, here a JSON array of rules under chaos.scenario. The
// service extracts the baggage with the standard OTel propagator and reads the
// scenario with its organisation's codec.
func ExampleScenarioFromContext() {
	carrier := propagation.MapCarrier{
		"baggage": "chaos.scenario=%5B%7B%22point%22%3A%22http%3Aratings%22%2C%22delay_ms%22%3A250%2C%22abort%22%3Atrue%7D%5D",
	}
	ctx := propagation.Baggage{}.Extract(context.Background(), carrier)

	s, ok, err := fit.ScenarioFromContext(ctx, jsonCodec{key: "chaos.scenario"})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(ok)
	fmt.Printf("%+v\n", s)
	// Output:
	// true
	// {Rules:[{Point:http:ratings Fault:{Delay:250ms Abort:true}}]}
}

// A gateway that can only set fixed values describes one rule with two static
// members.
func ExampleScenarioFromContext_staticMembers() {
	carrier := propagation.MapCarrier{
		"baggage": "chaos.target=payments,chaos.fault=abort",
	}
	ctx := propagation.Baggage{}.Extract(context.Background(), carrier)

	s, ok, err := fit.ScenarioFromContext(ctx, staticDecoder{targetKey: "chaos.target", faultKey: "chaos.fault"})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(ok)
	fmt.Printf("%+v\n", s)
	// Output:
	// true
	// {Rules:[{Point:payments Fault:{Delay:0s Abort:true}}]}
}
