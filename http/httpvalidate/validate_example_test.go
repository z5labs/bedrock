// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package httpvalidate

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
)

func ExampleRequest() {
	h := Request(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Hello, world!")
		}),
	)

	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal(err)
		return
	}

	s := &http.Server{
		Handler: h,
	}
	defer s.Shutdown(context.Background())

	go func() {
		s.Serve(ls)
	}()

	resp, err := http.Get(fmt.Sprintf("http://%s", ls.Addr()))
	if err != nil {
		log.Fatal(err)
		return
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
		return
	}

	fmt.Println(string(b))
	//Output: Hello, world!
}

func ExampleRequest_validationFailed() {
	h := Request(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Hello, world!")
		}),
		ForMethods(http.MethodGet),
	)

	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal(err)
		return
	}

	s := &http.Server{
		Handler: h,
	}
	defer s.Shutdown(context.Background())

	go func() {
		s.Serve(ls)
	}()

	resp, err := http.Post(fmt.Sprintf("http://%s", ls.Addr()), "application/json", nil)
	if err != nil {
		log.Fatal(err)
		return
	}
	defer resp.Body.Close()

	fmt.Println(resp.StatusCode)
	//Output: 405
}

func ExampleForMethods() {
	h := Request(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Hello, world!")
		}),
		ForMethods(http.MethodGet),
	)

	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal(err)
		return
	}

	s := &http.Server{
		Handler: h,
	}
	defer s.Shutdown(context.Background())

	go func() {
		s.Serve(ls)
	}()

	resp, err := http.Get(fmt.Sprintf("http://%s", ls.Addr()))
	if err != nil {
		log.Fatal(err)
		return
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
		return
	}

	fmt.Println(string(b))
	//Output: Hello, world!
}

func ExampleMinimumParams() {
	h := Request(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Hello, world!")
		}),
		MinimumParams("hello"),
	)

	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal(err)
		return
	}

	s := &http.Server{
		Handler: h,
	}
	defer s.Shutdown(context.Background())

	go func() {
		s.Serve(ls)
	}()

	resp, err := http.Get(fmt.Sprintf("http://%s?hello=world&good=bye", ls.Addr()))
	if err != nil {
		log.Fatal(err)
		return
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
		return
	}

	fmt.Println(string(b))
	//Output: Hello, world!
}

func ExampleExactParams() {
	h := Request(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Hello, world!")
		}),
		ExactParams("hello"),
	)

	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal(err)
		return
	}

	s := &http.Server{
		Handler: h,
	}
	defer s.Shutdown(context.Background())

	go func() {
		s.Serve(ls)
	}()

	resp, err := http.Get(fmt.Sprintf("http://%s?hello=world", ls.Addr()))
	if err != nil {
		log.Fatal(err)
		return
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
		return
	}

	fmt.Println(string(b))
	//Output: Hello, world!
}
