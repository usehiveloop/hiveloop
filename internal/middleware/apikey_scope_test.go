package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/middleware"
)

func TestRequireAPIKeyScopeOrJWT_AllowsMatchingScope(t *testing.T) {
	mw := middleware.RequireAPIKeyScopeOrJWT("credentials")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		KeyID:  "key-id",
		OrgID:  "org-id",
		Scopes: []string{"credentials"},
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAPIKeyScopeOrJWT_AllScopeGrantsAccess(t *testing.T) {
	mw := middleware.RequireAPIKeyScopeOrJWT("credentials")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		KeyID:  "key-id",
		OrgID:  "org-id",
		Scopes: []string{"all"},
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (all scope grants access), got %d", rr.Code)
	}
}

func TestRequireAPIKeyScopeOrJWT_DeniesWrongScope(t *testing.T) {
	mw := middleware.RequireAPIKeyScopeOrJWT("credentials")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called with wrong scope")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		KeyID:  "key-id",
		OrgID:  "org-id",
		Scopes: []string{"connect"},
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] != "api key lacks required scope: credentials" {
		t.Fatalf("unexpected error: %s", body["error"])
	}
}

func TestRequireAPIKeyScopeOrJWT_DeniesNoClaims(t *testing.T) {
	mw := middleware.RequireAPIKeyScopeOrJWT("credentials")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called without claims")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRequireAPIKeyScopeOrJWT_MultipleScopes(t *testing.T) {
	mw := middleware.RequireAPIKeyScopeOrJWT("tokens")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		KeyID:  "key-id",
		OrgID:  "org-id",
		Scopes: []string{"connect", "tokens"},
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (tokens in scopes list), got %d", rr.Code)
	}
}
