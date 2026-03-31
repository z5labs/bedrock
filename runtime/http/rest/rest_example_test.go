// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

type ExampleUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ExampleError struct {
	Message string `json:"message"`
}

func (e ExampleError) Error() string { return e.Message }

func wrapExampleError(err error) ExampleError {
	return ExampleError{Message: err.Error()}
}

func Example() {
	// Declare parameters.
	var userID = PathParam[string]("id")

	// Define endpoint with inside-out composition.
	ep := GET("/users/{id}", func(ctx context.Context, req Request[EmptyBody]) (ExampleUser, error) {
		id := ParamFrom(req, userID)
		if id == "missing" {
			return ExampleUser{}, ExampleError{Message: "user not found"}
		}
		return ExampleUser{ID: id, Name: "Alice"}, nil
	})
	ep = userID.Read(ep)
	ep = WriteJSON[ExampleUser](200, ep)
	ep = ErrorJSON[ExampleError](404, ep)
	route := CatchAll[ExampleError](500, wrapExampleError, ep)

	// Build the handler.
	handler := Build(
		Title("Example API"),
		Version("1.0.0"),
		route.Route(),
	)

	h, err := handler.Build(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	ts := httptest.NewServer(h)
	defer ts.Close()

	// Make a request to get a user.
	resp, err := http.Get(ts.URL + "/users/123")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()

	var user ExampleUser
	json.NewDecoder(resp.Body).Decode(&user)
	fmt.Printf("GET /users/123: %d %s\n", resp.StatusCode, user.Name)

	// Make a request for a missing user.
	resp2, err := http.Get(ts.URL + "/users/missing")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp2.Body.Close()

	var errBody ExampleError
	json.NewDecoder(resp2.Body).Decode(&errBody)
	fmt.Printf("GET /users/missing: %d %s\n", resp2.StatusCode, errBody.Message)

	// Get the OpenAPI spec.
	resp3, err := http.Get(ts.URL + "/openapi.json")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp3.Body.Close()

	specBytes, _ := io.ReadAll(resp3.Body)
	var spec map[string]any
	json.Unmarshal(specBytes, &spec)
	info := spec["info"].(map[string]any)
	fmt.Printf("OpenAPI spec: %s v%s\n", info["title"], info["version"])

	// Output:
	// GET /users/123: 200 Alice
	// GET /users/missing: 404 user not found
	// OpenAPI spec: Example API v1.0.0
}
