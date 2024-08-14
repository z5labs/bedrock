// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import "github.com/z5labs/bedrock/pkg/config/key"

// Map is an ordinary map[string]any but implements the Source interface.
type Map map[string]any

// Apply implements the Source interface. It recursively walks the underlying
// map to find key value pairs to set on the given store.
func (m Map) Apply(store Store) error {
	return walkMap(m, store, nil)
}

func walkMap(m map[string]any, store Store, chain key.Chain) error {
	for k, v := range m {
		switch x := v.(type) {
		case map[string]any:
			err := walkMap(x, store, append(chain, key.Name(k)))
			if err != nil {
				return err
			}
		default:
			err := store.Set(append(chain, key.Name(k)), x)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
