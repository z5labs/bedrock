// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package fit

import (
	"context"

	"go.opentelemetry.io/otel/baggage"
)

// Decoder reads a Scenario from OTel baggage and strips it. It owns every
// wire convention on the reading side: which members carry a Scenario, their
// grammar, version, size limit, expiry, point-name rules and trust.
//
// Decode reports ok == false when b carries no Scenario, and a non-nil error
// when b carries one that cannot be honoured. Strip returns b without the
// members Decode reads, and leaves every other member alone.
//
// Implementations must be safe for concurrent use.
type Decoder interface {
	Decode(b baggage.Baggage) (s Scenario, ok bool, err error)
	Strip(b baggage.Baggage) baggage.Baggage
}

// Encoder writes a Scenario into OTel baggage, replacing any Scenario b
// already carries. It owns every wire convention on the writing side.
//
// Implementations must be safe for concurrent use.
type Encoder interface {
	Encode(b baggage.Baggage, s Scenario) (baggage.Baggage, error)
}

// ContextWithScenario returns a copy of ctx whose baggage carries s, as
// written by e. On error it returns ctx unchanged.
func ContextWithScenario(ctx context.Context, e Encoder, s Scenario) (context.Context, error) {
	b, err := e.Encode(baggage.FromContext(ctx), s)
	if err != nil {
		return ctx, err
	}
	return baggage.ContextWithBaggage(ctx, b), nil
}

// ScenarioFromContext returns d's reading of the baggage in ctx.
func ScenarioFromContext(ctx context.Context, d Decoder) (Scenario, bool, error) {
	return d.Decode(baggage.FromContext(ctx))
}

// ContextWithoutScenario returns a copy of ctx whose baggage has been
// stripped by d. Call it where requests cross a trust boundary, in either
// direction.
func ContextWithoutScenario(ctx context.Context, d Decoder) context.Context {
	return baggage.ContextWithBaggage(ctx, d.Strip(baggage.FromContext(ctx)))
}
