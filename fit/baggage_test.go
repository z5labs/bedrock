// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package fit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/fit"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
)

// stubDecoder returns fixed results, so that tests can tell whether they are
// passed through unchanged.
type stubDecoder struct {
	s   fit.Scenario
	ok  bool
	err error
}

func (d stubDecoder) Decode(baggage.Baggage) (fit.Scenario, bool, error) {
	return d.s, d.ok, d.err
}

func (stubDecoder) Strip(b baggage.Baggage) baggage.Baggage { return b }

// failingEncoder fails every Encode with err.
type failingEncoder struct {
	err error
}

func (e failingEncoder) Encode(b baggage.Baggage, _ fit.Scenario) (baggage.Baggage, error) {
	return b, e.err
}

// contextWithBaggage returns a copy of ctx carrying the baggage header value
// header.
func contextWithBaggage(t *testing.T, ctx context.Context, header string) context.Context {
	t.Helper()

	b, err := baggage.Parse(header)
	require.NoError(t, err)
	return baggage.ContextWithBaggage(ctx, b)
}

// members returns the members of the baggage in ctx, keyed by name.
func members(ctx context.Context) map[string]string {
	m := make(map[string]string)
	for _, member := range baggage.FromContext(ctx).Members() {
		m[member.Key()] = member.Value()
	}
	return m
}

var (
	codec  = jsonCodec{key: "chaos.scenario"}
	static = staticDecoder{targetKey: "chaos.target", faultKey: "chaos.fault"}
)

func TestContextWithScenario(t *testing.T) {
	t.Parallel()

	t.Run("keeps every other baggage member", func(t *testing.T) {
		t.Parallel()

		ctx := contextWithBaggage(t, t.Context(), "fit=enabled,chaos.target=payments,tenant=acme;region=eu")
		want := fit.Scenario{Rules: []fit.Rule{{Point: "payments", Fault: fit.Fault{Abort: true}}}}

		ctx, err := fit.ContextWithScenario(ctx, codec, want)
		require.NoError(t, err)

		got := members(ctx)
		require.Len(t, got, 4)
		require.Equal(t, "enabled", got["fit"])
		require.Equal(t, "payments", got["chaos.target"])
		require.Equal(t, "acme", got["tenant"])
		require.Equal(t, []baggage.Property{mustProperty(t, "region", "eu")}, baggage.FromContext(ctx).Member("tenant").Properties())

		s, ok, err := fit.ScenarioFromContext(ctx, codec)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, want, s)
	})

	t.Run("replaces a Scenario the baggage already carries", func(t *testing.T) {
		t.Parallel()

		ctx, err := fit.ContextWithScenario(t.Context(), codec, fit.Scenario{Rules: []fit.Rule{{Point: "orders", Fault: fit.Fault{Abort: true}}}})
		require.NoError(t, err)

		want := fit.Scenario{Rules: []fit.Rule{{Point: "payments", Fault: fit.Fault{Delay: time.Second}}}}
		ctx, err = fit.ContextWithScenario(ctx, codec, want)
		require.NoError(t, err)

		require.Len(t, members(ctx), 1)
		s, ok, err := fit.ScenarioFromContext(ctx, codec)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, want, s)
	})

	t.Run("returns the original ctx and the encoder's error when Encode fails", func(t *testing.T) {
		t.Parallel()

		ctx := contextWithBaggage(t, t.Context(), "fit=enabled")
		encodeErr := errors.New("scenario too large")

		got, err := fit.ContextWithScenario(ctx, failingEncoder{err: encodeErr}, fit.Scenario{Rules: []fit.Rule{{Point: "*", Fault: fit.Fault{Abort: true}}}})
		require.ErrorIs(t, err, encodeErr)
		require.Same(t, ctx, got)
		require.Equal(t, map[string]string{"fit": "enabled"}, members(got))
	})
}

func mustProperty(t *testing.T, key, value string) baggage.Property {
	t.Helper()

	p, err := baggage.NewKeyValueProperty(key, value)
	require.NoError(t, err)
	return p
}

func TestScenarioFromContext(t *testing.T) {
	t.Parallel()

	t.Run("passes the decoder's results through unchanged", func(t *testing.T) {
		t.Parallel()

		decodeErr := errors.New("scenario expired")
		scenario := fit.Scenario{Rules: []fit.Rule{{Point: "payments", Fault: fit.Fault{Abort: true}}}}

		testCases := []struct {
			name    string
			decoder stubDecoder
		}{
			{name: "a Scenario", decoder: stubDecoder{s: scenario, ok: true}},
			{name: "no Scenario", decoder: stubDecoder{}},
			{name: "a Scenario that cannot be honoured", decoder: stubDecoder{ok: true, err: decodeErr}},
			{name: "an error without ok", decoder: stubDecoder{err: decodeErr}},
			{name: "a Scenario beside an error", decoder: stubDecoder{s: scenario, ok: true, err: decodeErr}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				s, ok, err := fit.ScenarioFromContext(t.Context(), tc.decoder)
				require.Equal(t, tc.decoder.s, s)
				require.Equal(t, tc.decoder.ok, ok)
				if tc.decoder.err == nil {
					require.NoError(t, err)
					return
				}
				require.ErrorIs(t, err, tc.decoder.err)
				require.Equal(t, tc.decoder.err, err)
			})
		}
	})

	t.Run("reports no Scenario when ctx carries no baggage", func(t *testing.T) {
		t.Parallel()

		s, ok, err := fit.ScenarioFromContext(t.Context(), codec)
		require.NoError(t, err)
		require.False(t, ok)
		require.Zero(t, s)
	})

	t.Run("reads static members into one aborting rule", func(t *testing.T) {
		t.Parallel()

		ctx := contextWithBaggage(t, t.Context(), "chaos.target=payments,chaos.fault=abort")

		s, ok, err := fit.ScenarioFromContext(ctx, static)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, fit.Scenario{Rules: []fit.Rule{{Point: "payments", Fault: fit.Fault{Abort: true}}}}, s)
	})

	t.Run("returns an error the caller matches with errors.Is", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name   string
			header string
		}{
			{name: "target without fault", header: "chaos.target=payments"},
			{name: "fault without target", header: "chaos.fault=abort"},
			{name: "unknown fault", header: "chaos.target=payments,chaos.fault=explode"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx := contextWithBaggage(t, t.Context(), tc.header)

				_, ok, err := fit.ScenarioFromContext(ctx, static)
				require.True(t, ok)
				require.ErrorIs(t, err, errStaticMembers)
			})
		}
	})
}

func TestContextWithoutScenario(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		decoder fit.Decoder
		header  string
		want    map[string]string
	}{
		{
			name:    "static members, beside neighbours and another tool's member",
			decoder: static,
			header:  "fit=enabled,chaos.target=payments,chaos.fault=abort,chaos.owner=sre,chaos.targets=orders",
			want: map[string]string{
				"fit":           "enabled",
				"chaos.owner":   "sre",
				"chaos.targets": "orders",
			},
		},
		{
			name:    "a JSON scenario, beside neighbours and another tool's member",
			decoder: codec,
			header:  "fit=enabled,chaos.scenario=%5B%5D,chaos.scenario.version=2,chaos.target=payments",
			want: map[string]string{
				"fit":                    "enabled",
				"chaos.scenario.version": "2",
				"chaos.target":           "payments",
			},
		},
		{
			name:    "baggage without a Scenario",
			decoder: codec,
			header:  "fit=enabled,chaos.target=payments",
			want: map[string]string{
				"fit":          "enabled",
				"chaos.target": "payments",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := contextWithBaggage(t, t.Context(), tc.header)

			stripped := fit.ContextWithoutScenario(ctx, tc.decoder)
			require.Equal(t, tc.want, members(stripped))

			_, ok, err := fit.ScenarioFromContext(stripped, tc.decoder)
			require.NoError(t, err)
			require.False(t, ok)
		})
	}

	t.Run("leaves the original ctx alone", func(t *testing.T) {
		t.Parallel()

		ctx := contextWithBaggage(t, t.Context(), "fit=enabled,chaos.target=payments,chaos.fault=abort")

		_ = fit.ContextWithoutScenario(ctx, static)

		_, ok, err := fit.ScenarioFromContext(ctx, static)
		require.NoError(t, err)
		require.True(t, ok)
	})
}

func TestScenario_baggageRoundTrip(t *testing.T) {
	t.Parallel()

	want := fit.Scenario{Rules: []fit.Rule{
		{Point: "http:ratings", Fault: fit.Fault{Delay: 250 * time.Millisecond, Abort: true}},
		{Point: "grpc:/payments.v1.Payments/Charge", Fault: fit.Fault{Abort: true}},
		{Point: "db: select * from users; -- naïve, =%"},
		{Point: "*", Fault: fit.Fault{Delay: 2 * time.Second}},
	}}

	ctx := contextWithBaggage(t, t.Context(), "fit=enabled")
	ctx, err := fit.ContextWithScenario(ctx, codec, want)
	require.NoError(t, err)

	carrier := propagation.MapCarrier{}
	propagation.Baggage{}.Inject(ctx, carrier)
	require.NotEmpty(t, carrier.Get("baggage"))

	extracted := propagation.Baggage{}.Extract(context.Background(), carrier)

	got, ok, err := fit.ScenarioFromContext(extracted, codec)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, want, got)
	require.Equal(t, "enabled", members(extracted)["fit"])
}
