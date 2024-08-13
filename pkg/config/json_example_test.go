// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/z5labs/bedrock/pkg/config/key"
)

func ExampleFromJson() {
	r := strings.NewReader(`{
	"hello": "world",
	"xs":[
	  1,
	  2,
	  3
	],
	"a":{
	  "b": 1.2
	}
}`)
	src := FromJson(r)

	var kvs []struct {
		key key.Keyer
		val any
	}
	store := storeFunc(func(k key.Keyer, v any) error {
		kvs = append(kvs, struct {
			key key.Keyer
			val any
		}{
			key: k,
			val: v,
		})
		return nil
	})
	err := src.Apply(store)
	if err != nil {
		fmt.Println(err)
		return
	}

	// key chains come from a map which is unordered thus
	// we need to sort the chains before printing and comparing output.
	slices.SortFunc(kvs, func(a, b struct {
		key key.Keyer
		val any
	}) int {
		return strings.Compare(a.key.Key(), b.key.Key())
	})

	for _, kv := range kvs {
		fmt.Println(kv.key.Key(), kv.val)
	}

	// Output: a.b 1.2
	// hello world
	// xs [1 2 3]

}
