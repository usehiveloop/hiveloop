package handler_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand/v2"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/crypto"
)

func withChiURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func randSuffix() string {
	return fmt.Sprintf("%x", rand.Uint64()) //nolint:gosec // tests, not security
}

func newTestKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	key := make([]byte, 32)
	b64 := base64.StdEncoding.EncodeToString(key)
	kms, err := crypto.NewAEADWrapper(context.Background(), b64, "test-kms")
	if err != nil {
		t.Fatalf("KMS: %v", err)
	}
	return kms
}
