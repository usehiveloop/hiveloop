package handler_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestOAuth_Callback_ProviderNotConfigured(t *testing.T) {
	h := newOAuthHarness(t) // no providers configured

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?code=abc&state=xyz", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 redirect, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=provider_not_configured") {
		t.Errorf("expected provider_not_configured error, got redirect to %s", loc)
	}
}

func TestOAuth_Callback_InvalidState(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?code=abc&state=wrong", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "correct"})
	req.AddCookie(&http.Cookie{Name: "oauth_verifier", Value: "some-verifier"})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=invalid_state") {
		t.Errorf("expected invalid_state error, got redirect to %s", loc)
	}
}

func TestOAuth_Callback_MissingState(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?code=abc&state=xyz", nil)
	// no oauth_state cookie
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=invalid_state") {
		t.Errorf("expected invalid_state error, got redirect to %s", loc)
	}
}

func TestOAuth_Callback_MissingVerifier(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?code=abc&state=mystate", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "mystate"})
	// no oauth_verifier cookie
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=missing_verifier") {
		t.Errorf("expected missing_verifier error, got redirect to %s", loc)
	}
}

func TestOAuth_Callback_MissingCode(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?state=mystate", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "mystate"})
	req.AddCookie(&http.Cookie{Name: "oauth_verifier", Value: "some-verifier"})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=missing_code") {
		t.Errorf("expected missing_code error, got redirect to %s", loc)
	}
}

func TestOAuth_Callback_ProviderError(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x/callback?state=mystate&error=access_denied", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "mystate"})
	req.AddCookie(&http.Cookie{Name: "oauth_verifier", Value: "some-verifier"})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=access_denied") {
		t.Errorf("expected access_denied error forwarded, got redirect to %s", loc)
	}
}

// ---------------------------------------------------------------------------
// Exchange endpoint tests
// ---------------------------------------------------------------------------

