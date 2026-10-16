// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package fit_test

import (
	"context"
	"fmt"
	"time"

	"github.com/z5labs/bedrock/fit"
	"go.opentelemetry.io/otel/propagation"
)

// A request carries only the ID of a scenario the service already holds, and
// a registry decoder resolves it. An unknown ID is reported through observe
// and injects nothing, so the request proceeds.
func ExampleEvaluator_Eval() {
	registry := registryDecoder{
		key: "acme-fault",
		scenarios: map[string]fit.Scenario{
			"scn-42": {Rules: []fit.Rule{
				{Point: "http:payments", Fault: fit.Fault{Delay: 250 * time.Millisecond, Abort: true}},
			}},
		},
	}
	observe := func(_ context.Context, ev fit.Evaluation) {
		if ev.Err != nil {
			fmt.Printf("observe: %s: %v\n", ev.Point, ev.Err)
			return
		}
		fmt.Printf("observe: %s: matched=%t fault=%+v\n", ev.Point, ev.Matched, ev.Fault)
	}

	// At startup.
	eval, err := fit.NewEvaluator(registry, time.Second, observe)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Per outbound call.
	for _, header := range []string{"acme-fault=scn-42", "acme-fault=scn-99"} {
		ctx := propagation.Baggage{}.Extract(context.Background(), propagation.MapCarrier{"baggage": header})
		f := eval.Eval(ctx, "http:payments")
		fmt.Printf("%s: %+v\n", header, f)
	}
	// Output:
	// observe: http:payments: matched=true fault={Delay:250ms Abort:true}
	// acme-fault=scn-42: {Delay:250ms Abort:true}
	// observe: http:payments: acme-fault: unknown scenario: scn-99
	// acme-fault=scn-99: {Delay:0s Abort:false}
}
