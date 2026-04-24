package handler_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// withChiURLParam attaches a chi URL parameter to the request's context so
// handlers that read chi.URLParam(r, "id") work when invoked directly
// (without routing through a chi.Router).
func withChiURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// randSuffix is a short random string for uniquifying org names in tests.
func randSuffix() string {
	return fmt.Sprintf("%x", rand.Uint64()) //nolint:gosec // tests, not security
}
