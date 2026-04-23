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

func TestOAuth_Login_ProviderNotConfigured(t *testing.T) {
	h := newOAuthHarness(t)

	for _, path := range []string{"/oauth/github", "/oauth/google", "/oauth/x"} {
		rr := h.doRequest(t, http.MethodGet, path, nil)
		if rr.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", path, rr.Code)
		}
	}
}

func TestOAuth_Login_RedirectsToProvider(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	tests := []struct {
		path     string
		wantHost string
	}{
		{"/oauth/github", "github.com"},
		{"/oauth/google", "accounts.google.com"},
		{"/oauth/x", "x.com"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusTemporaryRedirect {
			t.Errorf("%s: expected 307, got %d", tc.path, rr.Code)
			continue
		}

		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, tc.wantHost) {
			t.Errorf("%s: expected redirect to %s, got %s", tc.path, tc.wantHost, loc)
		}
	}
}

func TestOAuth_Login_SetsCookies(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	cookies := rr.Result().Cookies()
	var hasState, hasVerifier bool
	for _, c := range cookies {
		switch c.Name {
		case "oauth_state":
			hasState = true
			if c.Value == "" {
				t.Error("oauth_state cookie is empty")
			}
			if !c.HttpOnly {
				t.Error("oauth_state cookie should be HttpOnly")
			}
		case "oauth_verifier":
			hasVerifier = true
			if c.Value == "" {
				t.Error("oauth_verifier cookie is empty")
			}
			if !c.HttpOnly {
				t.Error("oauth_verifier cookie should be HttpOnly")
			}
		}
	}
	if !hasState {
		t.Error("missing oauth_state cookie")
	}
	if !hasVerifier {
		t.Error("missing oauth_verifier cookie")
	}
}

func TestOAuth_Login_PKCEInRedirectURL(t *testing.T) {
	h := newOAuthHarnessWithProviders(t)

	req := httptest.NewRequest(http.MethodGet, "/oauth/x", nil)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "code_challenge=") {
		t.Error("redirect URL missing code_challenge parameter")
	}
	if !strings.Contains(loc, "code_challenge_method=S256") {
		t.Error("redirect URL missing code_challenge_method=S256")
	}
}

// ---------------------------------------------------------------------------
// Callback endpoint tests
// ---------------------------------------------------------------------------

