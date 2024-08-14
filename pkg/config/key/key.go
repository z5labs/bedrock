// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package key

import (
	"strings"
)

type Keyer interface {
	Key() string
}

type Chain []Keyer

func (k Chain) Key() string {
	ss := make([]string, len(k))
	for i := range len(k) {
		ss[i] = k[i].Key()
	}
	return strings.Join(ss, ".")
}

type Name string

func (k Name) Key() string {
	return string(k)
}
