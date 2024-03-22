// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package configtmpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapEnv(t *testing.T) {
	t.Run("will ignore malformed pairs", func(t *testing.T) {
		t.Run("if there is no '=' separating the key and value", func(t *testing.T) {
			pairs := []string{"hello=world", "good bye"}
			env := mapEnv(pairs)
			if !assert.Less(t, len(env), len(pairs)) {
				return
			}
			if !assert.Contains(t, env, "hello") {
				return
			}
		})
	})
}
