// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEcho(t *testing.T) {
	t.Run("will echo message back", func(t *testing.T) {
		t.Run("always", func(t *testing.T) {
			e := Echo(slog.Default())

			req := `{"msg": "hello world"}`

			w := httptest.NewRecorder()
			r := httptest.NewRequest(
				http.MethodPost,
				"/echo",
				strings.NewReader(req),
			)
			r.Header.Set("Content-Type", "application/json")

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, http.StatusOK, resp.StatusCode) {
				return
			}

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}

			var echoResp EchoResponse
			err = json.Unmarshal(b, &echoResp)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello world", echoResp.Msg) {
				return
			}
		})
	})
}
