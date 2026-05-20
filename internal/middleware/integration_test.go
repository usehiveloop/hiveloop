package middleware_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_Auth_ValidToken(t *testing.T) {
	db := connectTestDB(t)
	ah := newAuthHelper(t)

	org, userJWT := ah.createTestOrg(t, db, "test-auth-valid", "admin")

	var gotOrg *model.Org
	handler := middleware.RequireAuth(&ah.privKey.PublicKey, ah.issuer, ah.audience)(
		middleware.ResolveOrgFromClaims(db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var ok bool
				gotOrg, ok = middleware.OrgFromContext(r.Context())
				if !ok {
					t.Fatal("org not found in context")
				}
				w.WriteHeader(http.StatusOK)
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+userJWT)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if gotOrg == nil || gotOrg.ID != org.ID {
		t.Fatalf("expected org ID %s, got %v", org.ID, gotOrg)
	}
	if gotOrg.Name != org.Name {
		t.Fatalf("expected org name %q, got %s", org.Name, gotOrg.Name)
	}
}

func TestIntegration_Auth_MissingToken(t *testing.T) {
	db := connectTestDB(t)
	ah := newAuthHelper(t)

	handler := middleware.RequireAuth(&ah.privKey.PublicKey, ah.issuer, ah.audience)(
		middleware.ResolveOrgFromClaims(db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called")
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_Auth_InvalidToken(t *testing.T) {
	db := connectTestDB(t)
	ah := newAuthHelper(t)

	handler := middleware.RequireAuth(&ah.privKey.PublicKey, ah.issuer, ah.audience)(
		middleware.ResolveOrgFromClaims(db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called")
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-xyz")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_Auth_InactiveOrg(t *testing.T) {
	db := connectTestDB(t)
	ah := newAuthHelper(t)

	org, userJWT := ah.createTestOrg(t, db, "test-auth-inactive", "admin")

	if err := db.Model(&org).Update("active", false).Error; err != nil {
		t.Fatalf("failed to deactivate org: %v", err)
	}

	handler := middleware.RequireAuth(&ah.privKey.PublicKey, ah.issuer, ah.audience)(
		middleware.ResolveOrgFromClaims(db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called for inactive org")
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+userJWT)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] != "organization is inactive" {
		t.Fatalf("unexpected error: %s", body["error"])
	}
}

func TestIntegration_Auth_WrongIssuerRejected(t *testing.T) {
	db := connectTestDB(t)
	ah := newAuthHelper(t)

	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("test-auth-issuer-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	wrongJWT, err := auth.IssueAccessToken(ah.privKey, "wrong-issuer", ah.audience, uuid.New().String(), orgID.String(), "admin", time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	handler := middleware.RequireAuth(&ah.privKey.PublicKey, ah.issuer, ah.audience)(
		middleware.ResolveOrgFromClaims(db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called with wrong issuer")
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+wrongJWT)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}
