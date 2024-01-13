// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package httpclient

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func serve(f func(http.ResponseWriter, *http.Request)) (addr string, stop func(), err error) {
	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", nil, err
	}
	addr = ls.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/", f)
	s := &http.Server{
		Handler: mux,
	}
	stop = func() {
		s.Shutdown(context.Background())
	}

	go func() {
		s.Serve(ls)
	}()
	return
}

func TestTimeout(t *testing.T) {
	t.Run("will timeout", func(t *testing.T) {
		t.Run("if the timeout is set to be greater than zero", func(t *testing.T) {
			timeout := 500 * time.Millisecond
			addr, stop, err := serve(func(w http.ResponseWriter, r *http.Request) {
				<-time.After(2 * timeout)
			})
			if !assert.Nil(t, err) {
				return
			}
			defer stop()

			client := New(Timeout(timeout))
			_, err = client.Get(fmt.Sprintf("http://%s/", addr))

			var nerr net.Error
			if !assert.ErrorAs(t, err, &nerr) {
				return
			}
			if !assert.True(t, nerr.Timeout()) {
				return
			}
		})
	})
}

func TestCircuitBreaker(t *testing.T) {

}

func TestRetry(t *testing.T) {

}
