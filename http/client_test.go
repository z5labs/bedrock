// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package http

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClient_Do(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if it fails to convert standard request to retryable request", func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "", new(bytes.Buffer))
			if !assert.Nil(t, err) {
				return
			}

			client := NewClient()
			resp, err := client.Do(req)
			if !assert.Nil(t, resp) {
				return
			}
			if !assert.Error(t, err) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {})
}
