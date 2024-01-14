package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

func ExampleRuntime_Run() {
	rt := NewRuntime(
		ListenOnPort(8080),
		HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "Hello, world")
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		rt.Run(ctx)
	}()

	resp, err := http.Get("http://localhost:8080")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(string(b))
	// Output: Hello, world
}
