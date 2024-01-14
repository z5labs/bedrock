package httpclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

func ExampleNew() {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{})

	c := New(
		LogHandler(h),
	)

	s := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		fmt.Println(err)
		return
	}

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		s.Serve(ls)
	}()

	resp, err := c.Get(fmt.Sprintf("http://%s", ls.Addr()))
	if err != nil {
		fmt.Println(err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = s.Shutdown(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}

	br := bufio.NewReader(&buf)
	line, _, err := br.ReadLine()
	if err != nil {
		fmt.Println(err)
		return
	}

	var log struct {
		Msg string `json:"msg"`
	}
	err = json.Unmarshal(line, &log)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(log.Msg)
	// Output: request sent
}

func ExampleNew_named() {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{})

	c := New(
		Name("example"),
		LogHandler(h),
	)

	s := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		fmt.Println(err)
		return
	}

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		s.Serve(ls)
	}()

	resp, err := c.Get(fmt.Sprintf("http://%s", ls.Addr()))
	if err != nil {
		fmt.Println(err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = s.Shutdown(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}

	br := bufio.NewReader(&buf)
	line, _, err := br.ReadLine()
	if err != nil {
		fmt.Println(err)
		return
	}

	var log struct {
		Name string `json:"http_client"`
		Msg  string `json:"msg"`
	}
	err = json.Unmarshal(line, &log)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(log.Name)
	fmt.Println(log.Msg)
	// Output: example
	// request sent
}
