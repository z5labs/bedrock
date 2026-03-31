// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test types

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CreateUserReq struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type NotFoundError struct {
	Message string `json:"message"`
}

func (e NotFoundError) Error() string { return e.Message }

type ValidationErr struct {
	Message string `json:"message"`
	Field   string `json:"field"`
}

func (e ValidationErr) Error() string { return e.Message }

type GenericError struct {
	Message string `json:"message"`
}

func (e GenericError) Error() string { return e.Message }

func wrapGenericError(err error) GenericError {
	return GenericError{Message: err.Error()}
}

// Tests

func TestBuild_ServesOpenAPISpec(t *testing.T) {
	var userID = PathParam[string]("id")

	ep := GET("/users/{id}", func(ctx context.Context, req Request[EmptyBody]) (User, error) {
		return User{ID: ParamFrom(req, userID)}, nil
	})
	ep = userID.Read(ep)
	ep = WriteJSON[User](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	handler := Build(
		Title("Test API"),
		Version("1.0.0"),
		route.Route(),
	)

	h, err := handler.Build(context.Background())
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var spec map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &spec)
	require.NoError(t, err)

	// Check basic spec structure.
	assert.Equal(t, "3.0.3", spec["openapi"])
	info := spec["info"].(map[string]any)
	assert.Equal(t, "Test API", info["title"])
	assert.Equal(t, "1.0.0", info["version"])

	// Check that paths are registered.
	paths := spec["paths"].(map[string]any)
	assert.Contains(t, paths, "/users/{id}")
}

func TestBuild_CustomSpecPath(t *testing.T) {
	ep := GET("/health", func(ctx context.Context, req Request[EmptyBody]) (map[string]string, error) {
		return map[string]string{"status": "ok"}, nil
	})
	ep = WriteJSON[map[string]string](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	handler := Build(
		SpecPath("/api/spec.json"),
		route.Route(),
	)

	h, err := handler.Build(context.Background())
	require.NoError(t, err)

	// Default path should not work.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Custom path should work.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/api/spec.json", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGET_WithPathParam(t *testing.T) {
	var userID = PathParam[string]("id")

	ep := GET("/users/{id}", func(ctx context.Context, req Request[EmptyBody]) (User, error) {
		id := ParamFrom(req, userID)
		return User{ID: id, Name: "Alice"}, nil
	})
	ep = userID.Read(ep)
	ep = WriteJSON[User](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var user User
	err := json.Unmarshal(w.Body.Bytes(), &user)
	require.NoError(t, err)
	assert.Equal(t, "123", user.ID)
	assert.Equal(t, "Alice", user.Name)
}

func TestGET_WithQueryParam(t *testing.T) {
	var search = QueryParam[string]("q")
	var page = QueryParam[int]("page", Optional(), DefaultValue(1))

	type SearchResult struct {
		Query string `json:"query"`
		Page  int    `json:"page"`
	}

	ep := GET("/search", func(ctx context.Context, req Request[EmptyBody]) (SearchResult, error) {
		return SearchResult{
			Query: ParamFrom(req, search),
			Page:  ParamFrom(req, page),
		}, nil
	})
	ep = search.Read(ep)
	ep = page.Read(ep)
	ep = WriteJSON[SearchResult](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/search?q=hello&page=3", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var result SearchResult
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Query)
	assert.Equal(t, 3, result.Page)
}

func TestGET_QueryParamDefault(t *testing.T) {
	var page = QueryParam[int]("page", Optional(), DefaultValue(1))

	type Result struct {
		Page int `json:"page"`
	}

	ep := GET("/items", func(ctx context.Context, req Request[EmptyBody]) (Result, error) {
		return Result{Page: ParamFrom(req, page)}, nil
	})
	ep = page.Read(ep)
	ep = WriteJSON[Result](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/items", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var result Result
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Page)
}

func TestPOST_WithJSONBody(t *testing.T) {
	ep := POST("/users", func(ctx context.Context, req Request[CreateUserReq]) (User, error) {
		body := req.Body()
		return User{ID: "new-id", Name: body.Name, Email: body.Email}, nil
	})
	ep = ReadJSON[CreateUserReq](ep)
	ep = WriteJSON[User](201, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	body := `{"name":"Bob","email":"bob@example.com"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusCreated, w.Code)

	var user User
	err := json.Unmarshal(w.Body.Bytes(), &user)
	require.NoError(t, err)
	assert.Equal(t, "new-id", user.ID)
	assert.Equal(t, "Bob", user.Name)
	assert.Equal(t, "bob@example.com", user.Email)
}

func TestPOST_InvalidJSONBody(t *testing.T) {
	ep := POST("/users", func(ctx context.Context, req Request[CreateUserReq]) (User, error) {
		return User{}, nil
	})
	ep = ReadJSON[CreateUserReq](ep)
	ep = WriteJSON[User](201, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader("not json"))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestErrorJSON_MatchesSpecificError(t *testing.T) {
	var userID = PathParam[string]("id")

	ep := GET("/users/{id}", func(ctx context.Context, req Request[EmptyBody]) (User, error) {
		id := ParamFrom(req, userID)
		if id == "missing" {
			return User{}, NotFoundError{Message: "user not found"}
		}
		return User{ID: id}, nil
	})
	ep = userID.Read(ep)
	ep = WriteJSON[User](200, ep)
	ep = ErrorJSON[NotFoundError](404, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	// Test not found error.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/users/missing", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var errResp NotFoundError
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Equal(t, "user not found", errResp.Message)
}

func TestErrorJSON_MultipleErrorTypes(t *testing.T) {
	var userID = PathParam[string]("id")

	ep := GET("/users/{id}", func(ctx context.Context, req Request[EmptyBody]) (User, error) {
		id := ParamFrom(req, userID)
		switch id {
		case "invalid":
			return User{}, ValidationErr{Message: "invalid id", Field: "id"}
		case "missing":
			return User{}, NotFoundError{Message: "not found"}
		default:
			return User{ID: id}, nil
		}
	})
	ep = userID.Read(ep)
	ep = WriteJSON[User](200, ep)
	ep = ErrorJSON[ValidationErr](422, ep)
	ep = ErrorJSON[NotFoundError](404, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	// Validation error -> 422.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/users/invalid", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, 422, w.Code)

	// Not found -> 404.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/users/missing", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Success -> 200.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/users/123", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCatchAll_HandlesUnmatchedErrors(t *testing.T) {
	ep := GET("/fail", func(ctx context.Context, req Request[EmptyBody]) (User, error) {
		return User{}, io.ErrUnexpectedEOF // not a declared error type
	})
	ep = WriteJSON[User](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/fail", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPathParam_RequiredValidation(t *testing.T) {
	var userID = PathParam[string]("id", MinLength(3), MaxLength(10))

	ep := GET("/users/{id}", func(ctx context.Context, req Request[EmptyBody]) (User, error) {
		return User{ID: ParamFrom(req, userID)}, nil
	})
	ep = userID.Read(ep)
	ep = WriteJSON[User](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	// Too short.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/users/ab", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Valid.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/users/abc", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestQueryParam_PatternValidation(t *testing.T) {
	var code = QueryParam[string]("code", Required(), Pattern(`^[A-Z]{3}$`))

	type Result struct {
		Code string `json:"code"`
	}

	ep := GET("/validate", func(ctx context.Context, req Request[EmptyBody]) (Result, error) {
		return Result{Code: ParamFrom(req, code)}, nil
	})
	ep = code.Read(ep)
	ep = WriteJSON[Result](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	// Invalid pattern.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/validate?code=abc", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Valid pattern.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/validate?code=ABC", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestQueryParam_NumericConstraints(t *testing.T) {
	var page = QueryParam[int]("page", Required(), Minimum(1), Maximum(100))

	type Result struct {
		Page int `json:"page"`
	}

	ep := GET("/items", func(ctx context.Context, req Request[EmptyBody]) (Result, error) {
		return Result{Page: ParamFrom(req, page)}, nil
	})
	ep = page.Read(ep)
	ep = WriteJSON[Result](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	// Below minimum.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/items?page=0", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Above maximum.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/items?page=101", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Valid.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/items?page=50", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDELETE_Endpoint(t *testing.T) {
	var userID = PathParam[string]("id")

	type DeleteResp struct {
		Deleted bool `json:"deleted"`
	}

	ep := DELETE("/users/{id}", func(ctx context.Context, req Request[EmptyBody]) (DeleteResp, error) {
		_ = ParamFrom(req, userID)
		return DeleteResp{Deleted: true}, nil
	})
	ep = userID.Read(ep)
	ep = WriteJSON[DeleteResp](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/users/123", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DeleteResp
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Deleted)
}

func TestPUT_WithBodyAndPathParam(t *testing.T) {
	var userID = PathParam[string]("id")

	ep := PUT("/users/{id}", func(ctx context.Context, req Request[CreateUserReq]) (User, error) {
		id := ParamFrom(req, userID)
		body := req.Body()
		return User{ID: id, Name: body.Name, Email: body.Email}, nil
	})
	ep = userID.Read(ep)
	ep = ReadJSON[CreateUserReq](ep)
	ep = WriteJSON[User](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	body := `{"name":"Updated","email":"updated@example.com"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/users/123", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var user User
	err := json.Unmarshal(w.Body.Bytes(), &user)
	require.NoError(t, err)
	assert.Equal(t, "123", user.ID)
	assert.Equal(t, "Updated", user.Name)
}

func TestEndpointMetadata_InSpec(t *testing.T) {
	ep := GET("/health", func(ctx context.Context, req Request[EmptyBody]) (map[string]string, error) {
		return map[string]string{"status": "ok"}, nil
	})
	ep = WriteJSON[map[string]string](200, ep)
	ep = Summary("Health check", ep)
	ep = EndpointDescription("Returns the health status of the service", ep)
	ep = Tags([]string{"monitoring"}, ep)
	ep = OperationID("healthCheck", ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	handler := Build(
		Title("Test API"),
		Version("1.0.0"),
		route.Route(),
	)

	h, err := handler.Build(context.Background())
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	h.ServeHTTP(w, r)

	var spec map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &spec)
	require.NoError(t, err)

	paths := spec["paths"].(map[string]any)
	healthPath := paths["/health"].(map[string]any)
	getOp := healthPath["get"].(map[string]any)

	assert.Equal(t, "Health check", getOp["summary"])
	assert.Equal(t, "Returns the health status of the service", getOp["description"])
	assert.Equal(t, "healthCheck", getOp["operationId"])
	tags := getOp["tags"].([]any)
	assert.Contains(t, tags, "monitoring")
}

func TestOpenAPISpec_ContainsParams(t *testing.T) {
	var userID = PathParam[string]("id", ParamDescription("User identifier"))

	ep := GET("/users/{id}", func(ctx context.Context, req Request[EmptyBody]) (User, error) {
		return User{ID: ParamFrom(req, userID)}, nil
	})
	ep = userID.Read(ep)
	ep = WriteJSON[User](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	handler := Build(Title("Test"), Version("1.0.0"), route.Route())
	h, err := handler.Build(context.Background())
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	h.ServeHTTP(w, r)

	var spec map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &spec)
	require.NoError(t, err)

	paths := spec["paths"].(map[string]any)
	userPath := paths["/users/{id}"].(map[string]any)
	getOp := userPath["get"].(map[string]any)
	params := getOp["parameters"].([]any)

	require.Len(t, params, 1)
	param := params[0].(map[string]any)
	assert.Equal(t, "id", param["name"])
	assert.Equal(t, "path", param["in"])
	assert.Equal(t, true, param["required"])
	assert.Equal(t, "User identifier", param["description"])
}

func TestOpenAPISpec_ContainsResponses(t *testing.T) {
	ep := GET("/users", func(ctx context.Context, req Request[EmptyBody]) ([]User, error) {
		return nil, nil
	})
	ep = WriteJSON[[]User](200, ep)
	ep = ErrorJSON[NotFoundError](404, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	handler := Build(Title("Test"), Version("1.0.0"), route.Route())
	h, err := handler.Build(context.Background())
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	h.ServeHTTP(w, r)

	var spec map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &spec)
	require.NoError(t, err)

	paths := spec["paths"].(map[string]any)
	usersPath := paths["/users"].(map[string]any)
	getOp := usersPath["get"].(map[string]any)
	responses := getOp["responses"].(map[string]any)

	assert.Contains(t, responses, "200")
	assert.Contains(t, responses, "404")
	assert.Contains(t, responses, "500")
}

func TestWriteBinary(t *testing.T) {
	ep := GET("/file", func(ctx context.Context, req Request[EmptyBody]) (io.Reader, error) {
		return strings.NewReader("binary content"), nil
	})
	ep = WriteBinary(200, "application/octet-stream", ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/file", nil)
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "binary content", w.Body.String())
}

func TestMultipleRoutes(t *testing.T) {
	var userID = PathParam[string]("id")

	ep1 := GET("/users/{id}", func(ctx context.Context, req Request[EmptyBody]) (User, error) {
		return User{ID: ParamFrom(req, userID)}, nil
	})
	ep1 = userID.Read(ep1)
	ep1 = WriteJSON[User](200, ep1)
	route1 := CatchAll[GenericError](500, wrapGenericError, ep1)

	ep2 := POST("/users", func(ctx context.Context, req Request[CreateUserReq]) (User, error) {
		body := req.Body()
		return User{ID: "new", Name: body.Name}, nil
	})
	ep2 = ReadJSON[CreateUserReq](ep2)
	ep2 = WriteJSON[User](201, ep2)
	route2 := CatchAll[GenericError](500, wrapGenericError, ep2)

	handler := Build(
		Title("Multi Route API"),
		Version("1.0.0"),
		route1.Route(),
		route2.Route(),
	)

	h, err := handler.Build(context.Background())
	require.NoError(t, err)

	// GET /users/123
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)

	// POST /users
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name":"Bob"}`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Both should appear in spec.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	h.ServeHTTP(w, r)

	var spec map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &spec)
	require.NoError(t, err)

	paths := spec["paths"].(map[string]any)
	assert.Contains(t, paths, "/users/{id}")
	assert.Contains(t, paths, "/users")
}

func TestQueryParam_RequiredMissing(t *testing.T) {
	var q = QueryParam[string]("q", Required())

	type Result struct {
		Q string `json:"q"`
	}

	ep := GET("/search", func(ctx context.Context, req Request[EmptyBody]) (Result, error) {
		return Result{Q: ParamFrom(req, q)}, nil
	})
	ep = q.Read(ep)
	ep = WriteJSON[Result](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	// Missing required param.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/search", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestQueryParam_InvalidType(t *testing.T) {
	var page = QueryParam[int]("page", Required())

	type Result struct {
		Page int `json:"page"`
	}

	ep := GET("/items", func(ctx context.Context, req Request[EmptyBody]) (Result, error) {
		return Result{Page: ParamFrom(req, page)}, nil
	})
	ep = page.Read(ep)
	ep = WriteJSON[Result](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	// Invalid int.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/items?page=abc", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHeaderParam(t *testing.T) {
	var apiKey = HeaderParam[string]("X-API-Key", Required())

	type Result struct {
		Key string `json:"key"`
	}

	ep := GET("/secure", func(ctx context.Context, req Request[EmptyBody]) (Result, error) {
		return Result{Key: ParamFrom(req, apiKey)}, nil
	})
	ep = apiKey.Read(ep)
	ep = WriteJSON[Result](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	// With header.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/secure", nil)
	r.Header.Set("X-API-Key", "secret123")
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)

	var result Result
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "secret123", result.Key)

	// Without header.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/secure", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPATCH_Endpoint(t *testing.T) {
	var id = PathParam[string]("id")

	type PatchBody struct {
		Name string `json:"name"`
	}

	ep := PATCH("/users/{id}", func(ctx context.Context, req Request[PatchBody]) (User, error) {
		return User{ID: ParamFrom(req, id), Name: req.Body().Name}, nil
	})
	ep = id.Read(ep)
	ep = ReadJSON[PatchBody](ep)
	ep = WriteJSON[User](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/users/123", strings.NewReader(`{"name":"Patched"}`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var user User
	err := json.Unmarshal(w.Body.Bytes(), &user)
	require.NoError(t, err)
	assert.Equal(t, "123", user.ID)
	assert.Equal(t, "Patched", user.Name)
}

func TestEnumValidation(t *testing.T) {
	var status = QueryParam[string]("status", Required(), Enum("active", "inactive", "pending"))

	type Result struct {
		Status string `json:"status"`
	}

	ep := GET("/filter", func(ctx context.Context, req Request[EmptyBody]) (Result, error) {
		return Result{Status: ParamFrom(req, status)}, nil
	})
	ep = status.Read(ep)
	ep = WriteJSON[Result](200, ep)
	route := CatchAll[GenericError](500, wrapGenericError, ep)

	h := buildAndServe(t, route)

	// Valid enum.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/filter?status=active", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)

	// Invalid enum.
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/filter?status=deleted", nil)
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Helper

func buildAndServe(t *testing.T, routes ...Route) http.Handler {
	t.Helper()

	opts := []Option{Title("Test"), Version("1.0.0")}
	for _, r := range routes {
		opts = append(opts, r.Route())
	}

	handler := Build(opts...)
	h, err := handler.Build(context.Background())
	require.NoError(t, err)
	return h
}
