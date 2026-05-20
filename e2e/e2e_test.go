package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/token"
)

func TestE2E_CredentialLifecycle(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key-12345")
	if cred.ID == uuid.Nil {
		t.Fatal("credential not created")
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}
	creds := decodePaginatedList(t, rr)
	found := false
	for _, c := range creds {
		if c["id"] == cred.ID.String() {
			found = true
		}
	}
	if !found {
		t.Fatal("created credential not in list")
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/credentials/"+cred.ID.String(), nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	body := fmt.Sprintf(`{"credential_id":%q,"ttl":"1h"}`, cred.ID.String())
	req = httptest.NewRequest(http.MethodPost, "/v1/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("mint after revoke: expected 404, got %d", rr.Code)
	}
}

func TestE2E_TokenLifecycle(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)
	cred := h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-fake-key-12345")

	tok := h.mintToken(t, org, cred.ID)
	if !strings.HasPrefix(tok, "ptok_") {
		t.Fatalf("expected ptok_ prefix, got %s", tok[:10])
	}

	jwtStr := strings.TrimPrefix(tok, "ptok_")
	claims, err := token.Validate(h.signingKey, jwtStr)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/tokens/"+claims.ID, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke token: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	proxyPath := "/v1/proxy/v1/chat/completions"
	rr = h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(`{}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("proxy with revoked token: expected 401, got %d", rr.Code)
	}
}
