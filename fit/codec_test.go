// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package fit_test

// The codecs in this file stand in for an organisation's own. Their baggage
// keys and grammars are examples, not conventions of fit.

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/z5labs/bedrock/fit"
	"go.opentelemetry.io/otel/baggage"
)

// jsonRule is one fit.Rule as the JSON codec writes it.
type jsonRule struct {
	Point   string `json:"point"`
	DelayMS int64  `json:"delay_ms,omitempty"`
	Abort   bool   `json:"abort,omitempty"`
}

// jsonCodec carries a whole Scenario as a JSON array of rules in a single
// baggage member, such as one an Envoy edge writes with a static header:
//
//	baggage: chaos.scenario=%5B%7B%22point%22%3A%22http%3Aratings%22%2C%22delay_ms%22%3A250%2C%22abort%22%3Atrue%7D%5D
//
// It is both a fit.Decoder and a fit.Encoder.
type jsonCodec struct {
	key string
}

var (
	_ fit.Decoder = jsonCodec{}
	_ fit.Encoder = jsonCodec{}
)

func (c jsonCodec) Decode(b baggage.Baggage) (fit.Scenario, bool, error) {
	m := b.Member(c.key)
	if m.Key() == "" {
		return fit.Scenario{}, false, nil
	}

	var rules []jsonRule
	if err := json.Unmarshal([]byte(m.Value()), &rules); err != nil {
		return fit.Scenario{}, true, fmt.Errorf("decode baggage member %s: %w", c.key, err)
	}

	s := fit.Scenario{Rules: make([]fit.Rule, 0, len(rules))}
	for _, r := range rules {
		s.Rules = append(s.Rules, fit.Rule{
			Point: r.Point,
			Fault: fit.Fault{
				Delay: time.Duration(r.DelayMS) * time.Millisecond,
				Abort: r.Abort,
			},
		})
	}
	return s, true, nil
}

func (c jsonCodec) Strip(b baggage.Baggage) baggage.Baggage {
	return b.DeleteMember(c.key)
}

func (c jsonCodec) Encode(b baggage.Baggage, s fit.Scenario) (baggage.Baggage, error) {
	rules := make([]jsonRule, 0, len(s.Rules))
	for _, r := range s.Rules {
		if r.Fault.Delay%time.Millisecond != 0 {
			return b, fmt.Errorf("encode rule for %q: delay %v is not a whole number of milliseconds", r.Point, r.Fault.Delay)
		}
		rules = append(rules, jsonRule{
			Point:   r.Point,
			DelayMS: r.Fault.Delay.Milliseconds(),
			Abort:   r.Fault.Abort,
		})
	}

	value, err := json.Marshal(rules)
	if err != nil {
		return b, fmt.Errorf("encode scenario: %w", err)
	}
	m, err := baggage.NewMemberRaw(c.key, string(value))
	if err != nil {
		return b, fmt.Errorf("encode baggage member %s: %w", c.key, err)
	}
	return b.SetMember(m)
}

// errStaticMembers is the error staticDecoder returns for members it cannot
// honour.
var errStaticMembers = errors.New("invalid static scenario members")

// staticDecoder builds a single Rule from two members holding fixed values,
// for a gateway that can only set static baggage:
//
//	baggage: chaos.target=payments,chaos.fault=abort
//
// The fault is either "abort" or a delay such as "250ms". staticDecoder is a
// fit.Decoder only: the edge that writes these members needs no Encoder.
type staticDecoder struct {
	targetKey string
	faultKey  string
}

var _ fit.Decoder = staticDecoder{}

func (d staticDecoder) Decode(b baggage.Baggage) (fit.Scenario, bool, error) {
	target := b.Member(d.targetKey)
	fault := b.Member(d.faultKey)
	if target.Key() == "" && fault.Key() == "" {
		return fit.Scenario{}, false, nil
	}
	if target.Key() == "" || fault.Key() == "" {
		return fit.Scenario{}, true, fmt.Errorf("%w: %s and %s must be set together", errStaticMembers, d.targetKey, d.faultKey)
	}

	var f fit.Fault
	if fault.Value() == "abort" {
		f.Abort = true
	} else {
		delay, err := time.ParseDuration(fault.Value())
		if err != nil {
			return fit.Scenario{}, true, fmt.Errorf("%w: %s: %w", errStaticMembers, d.faultKey, err)
		}
		f.Delay = delay
	}
	return fit.Scenario{Rules: []fit.Rule{{Point: target.Value(), Fault: f}}}, true, nil
}

func (d staticDecoder) Strip(b baggage.Baggage) baggage.Baggage {
	return b.DeleteMember(d.targetKey).DeleteMember(d.faultKey)
}
